package beads

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gastownhall/gascity/internal/events"
)

const (
	hqExpiresAtMetadataKey = "expires_at"
	hqExpiresAtMetadataAlt = "gc.expires_at"
	hqClosedAtMetadataKey  = "closed_at"
	hqClosedAtMetadataAlt  = "gc.closed_at"

	hqDefaultMainTierBackstopTTL  = 24 * time.Hour
	hqDefaultEphemeralBackstopTTL = 10 * time.Minute
	hqDefaultLeakThreshold        = 5
)

// HQStore is a dormant, snapshot-backed in-process Store implementation for the
// coordination-store migration experiments. Writes mutate an in-memory indexed
// core with no per-write fsync; durability comes from an async background
// snapshotter that periodically serializes the whole store to a gzip-compressed
// JSONL file and publishes it via atomic rename. It is not wired into live city
// storage; callers must opt in by opening it directly.
type HQStore struct {
	mu sync.RWMutex

	dir    string
	prefix string
	seq    int

	closed bool

	main      map[string]Bead
	wisps     map[string]Bead
	order     []string
	orderSeen map[string]bool
	deps      []Dep
	mainIdx   hqTierIndex
	wispIdx   hqTierIndex

	retentionQueue       []retentionEntry
	mainTierBackstopTTL  time.Duration
	ephemeralBackstopTTL time.Duration
	leakThreshold        int
	leakRecorder         events.Recorder

	ttlInterval time.Duration
	ttlStop     chan struct{}
	ttlDone     chan struct{}

	snapshotInterval time.Duration
	snapStop         chan struct{}
	snapDone         chan struct{}
	snapWriteMu      sync.Mutex // serializes concurrent snapshot writers
	snapErrMu        sync.Mutex
	snapErr          error
}

type retentionEntry struct {
	id          string
	deleteAfter time.Time
}

type hqStoreOptions struct {
	prefix               string
	ttlInterval          time.Duration
	snapshotInterval     time.Duration
	mainTierBackstopTTL  time.Duration
	ephemeralBackstopTTL time.Duration
	leakThreshold        int
	leakRecorder         events.Recorder
}

// HQStoreOption customizes OpenHQStore.
type HQStoreOption func(*hqStoreOptions)

// WithHQStoreTTLInterval starts a background TTL sweeper at the given interval.
// A non-positive interval leaves TTL purge explicit via PurgeExpired.
func WithHQStoreTTLInterval(d time.Duration) HQStoreOption {
	return func(o *hqStoreOptions) {
		o.ttlInterval = d
	}
}

// WithHQStoreIDPrefix sets the generated ID prefix. Empty keeps the default.
func WithHQStoreIDPrefix(prefix string) HQStoreOption {
	return func(o *hqStoreOptions) {
		if prefix != "" {
			o.prefix = prefix
		}
	}
}

// WithHQStoreSnapshotInterval sets the background snapshot cadence. A
// non-positive interval disables periodic snapshots; Shutdown still flushes a
// final snapshot so an orderly close is always durable.
func WithHQStoreSnapshotInterval(d time.Duration) HQStoreOption {
	return func(o *hqStoreOptions) {
		o.snapshotInterval = d
	}
}

// WithHQStoreMainTierBackstopTTL sets the backstop cutoff for closed main-tier
// beads. Closed main-tier beads older than this window are eligible for
// PurgeBackstop reclaim.
func WithHQStoreMainTierBackstopTTL(d time.Duration) HQStoreOption {
	return func(o *hqStoreOptions) {
		o.mainTierBackstopTTL = d
	}
}

// WithHQStoreEphemeralBackstopTTL sets the backstop cutoff for closed ephemeral
// beads. Closed wisps older than this window are eligible for PurgeBackstop
// reclaim.
func WithHQStoreEphemeralBackstopTTL(d time.Duration) HQStoreOption {
	return func(o *hqStoreOptions) {
		o.ephemeralBackstopTTL = d
	}
}

// WithHQStoreLeakDetector wires leak detection configuration for the backstop
// sweeper. A non-positive threshold disables the detector; a nil recorder
// suppresses event emission.
func WithHQStoreLeakDetector(threshold int, rec events.Recorder) HQStoreOption {
	return func(o *hqStoreOptions) {
		if threshold < 0 {
			threshold = 0
		}
		o.leakThreshold = threshold
		o.leakRecorder = rec
	}
}

// OpenHQStore opens or creates a dormant HQStore rooted at dir. If a snapshot
// is present it is loaded to rebuild in-memory state and indexes.
func OpenHQStore(dir string, opts ...HQStoreOption) (*HQStore, error) {
	cfg := hqStoreOptions{
		prefix:               "hq",
		snapshotInterval:     hqDefaultSnapshotInterval,
		mainTierBackstopTTL:  hqDefaultMainTierBackstopTTL,
		ephemeralBackstopTTL: hqDefaultEphemeralBackstopTTL,
		leakThreshold:        hqDefaultLeakThreshold,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("opening hqstore: %w", err)
	}

	store := &HQStore{
		dir:                  dir,
		prefix:               cfg.prefix,
		ttlInterval:          cfg.ttlInterval,
		snapshotInterval:     cfg.snapshotInterval,
		mainTierBackstopTTL:  cfg.mainTierBackstopTTL,
		ephemeralBackstopTTL: cfg.ephemeralBackstopTTL,
		leakThreshold:        cfg.leakThreshold,
		leakRecorder:         cfg.leakRecorder,
	}
	store.resetCoreLocked()

	if err := store.loadSnapshot(); err != nil {
		return nil, err
	}
	store.startSnapshotter()
	store.startTTLSweeper()
	return store, nil
}

// Shutdown stops the background goroutines, flushes a final snapshot, and marks
// the store closed. It is idempotent.
func (s *HQStore) Shutdown() error {
	s.stopTTLSweeper()
	s.stopSnapshotter()

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	// Final flush after marking closed: writeSnapshot takes only a read lock
	// internally via ExportAll, and no further writes can land because callers
	// hit ensureOpenLocked. snapWriteMu guards against a late periodic flush.
	if err := s.writeSnapshot(); err != nil {
		return fmt.Errorf("shutting down hqstore: %w", err)
	}
	return nil
}

// Ping verifies that the store is open.
func (s *HQStore) Ping() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return fmt.Errorf("pinging hqstore: closed")
	}
	return nil
}

// Tx executes fn against the HQStore write surface.
func (s *HQStore) Tx(_ string, fn func(tx Tx) error) error {
	return runSequentialTx(s, fn)
}

func (s *HQStore) ensureOpenLocked() error {
	if s.closed {
		return fmt.Errorf("hqstore is closed")
	}
	return nil
}
