package beads

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/events"
)

func TestHQStoreTTLSweepRunsExpiredRetentionThenBackstop(t *testing.T) {
	store := openInternalHQStoreForTest(t,
		WithHQStoreSnapshotInterval(0),
		WithHQStoreMainTierBackstopTTL(time.Hour),
		WithHQStoreEphemeralBackstopTTL(time.Hour),
		WithHQStoreLeakDetector(0, events.NewFake()),
	)
	old := time.Now().Add(-2 * time.Hour)

	expiredWisp := createClosedInternalHQBead(t, store, Bead{
		Title:     "expired-wisp",
		Ephemeral: true,
		Metadata: map[string]string{
			hqExpiresAtMetadataKey: time.Now().Add(-time.Minute).Format(time.RFC3339Nano),
		},
	}, old)
	retainedMain := createRetainedInternalHQBead(t, store, Bead{Title: "retained-main"}, old, time.Now().Add(-time.Minute))
	orphanMain := createClosedInternalHQBead(t, store, Bead{Title: "orphan-main"}, old)

	result := store.runTTLSweep()

	if result.Expired != 1 {
		t.Fatalf("Expired = %d, want 1", result.Expired)
	}
	if result.Retention != 1 {
		t.Fatalf("Retention = %d, want 1", result.Retention)
	}
	if result.Backstop.Main != 1 || result.Backstop.Ephemeral != 0 {
		t.Fatalf("Backstop = main:%d ephemeral:%d, want main:1 ephemeral:0", result.Backstop.Main, result.Backstop.Ephemeral)
	}
	for _, id := range []string{expiredWisp.ID, retainedMain.ID, orphanMain.ID} {
		if _, err := store.Get(id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Get(%q) error = %v, want ErrNotFound", id, err)
		}
	}
}

func TestHQStoreBackstopLeakDetectorEmitsTypedEventAboveThreshold(t *testing.T) {
	rec := events.NewFake()
	store := openInternalHQStoreForTest(t,
		WithHQStoreSnapshotInterval(0),
		WithHQStoreMainTierBackstopTTL(time.Hour),
		WithHQStoreEphemeralBackstopTTL(time.Hour),
		WithHQStoreLeakDetector(2, rec),
	)
	old := time.Now().Add(-2 * time.Hour)
	createClosedInternalHQBead(t, store, Bead{Title: "main-a"}, old)
	createClosedInternalHQBead(t, store, Bead{Title: "main-b"}, old)
	createClosedInternalHQBead(t, store, Bead{Title: "wisp", Ephemeral: true}, old)

	result := store.runTTLSweep()

	if result.Backstop.Total() != 3 {
		t.Fatalf("backstop total = %d, want 3", result.Backstop.Total())
	}
	if len(rec.Events) != 1 {
		t.Fatalf("events recorded = %d, want 1: %+v", len(rec.Events), rec.Events)
	}
	ev := rec.Events[0]
	if ev.Type != events.StoreBackstopLeakDetected {
		t.Fatalf("event type = %q, want %q", ev.Type, events.StoreBackstopLeakDetected)
	}
	if ev.Actor != "controller" {
		t.Fatalf("event actor = %q, want controller", ev.Actor)
	}
	if ev.Subject != store.dir {
		t.Fatalf("event subject = %q, want %q", ev.Subject, store.dir)
	}
	var payload events.StoreBackstopLeakDetectedPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.MainCount != 2 || payload.EphemeralCount != 1 || payload.TotalCount != 3 || payload.Threshold != 2 {
		t.Fatalf("payload = %+v, want main=2 ephemeral=1 total=3 threshold=2", payload)
	}
}

func TestHQStoreBackstopLeakDetectorSilentAtOrBelowThreshold(t *testing.T) {
	rec := events.NewFake()
	store := openInternalHQStoreForTest(t,
		WithHQStoreSnapshotInterval(0),
		WithHQStoreMainTierBackstopTTL(time.Hour),
		WithHQStoreEphemeralBackstopTTL(time.Hour),
		WithHQStoreLeakDetector(3, rec),
	)
	old := time.Now().Add(-2 * time.Hour)
	createClosedInternalHQBead(t, store, Bead{Title: "main-a"}, old)
	createClosedInternalHQBead(t, store, Bead{Title: "main-b"}, old)
	createClosedInternalHQBead(t, store, Bead{Title: "wisp", Ephemeral: true}, old)

	result := store.runTTLSweep()

	if result.Backstop.Total() != 3 {
		t.Fatalf("backstop total = %d, want 3", result.Backstop.Total())
	}
	if len(rec.Events) != 0 {
		t.Fatalf("events recorded = %d, want 0: %+v", len(rec.Events), rec.Events)
	}
}

func TestHQStoreBackstopReclaimLogsNormalAndLeakReclaimDistinctly(t *testing.T) {
	oldLogf := hqStoreTTLLogf
	var logs []string
	hqStoreTTLLogf = func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	}
	t.Cleanup(func() { hqStoreTTLLogf = oldLogf })

	normal := openInternalHQStoreForTest(t,
		WithHQStoreSnapshotInterval(0),
		WithHQStoreMainTierBackstopTTL(time.Hour),
		WithHQStoreEphemeralBackstopTTL(time.Hour),
		WithHQStoreLeakDetector(5, events.NewFake()),
	)
	createClosedInternalHQBead(t, normal, Bead{Title: "normal"}, time.Now().Add(-2*time.Hour))
	normal.runTTLSweep()

	if !joinedLogsContain(logs, "backstop reclaimed") {
		t.Fatalf("normal reclaim logs = %q, want backstop reclaimed", logs)
	}
	if joinedLogsContain(logs, "backstop leak detected") {
		t.Fatalf("normal reclaim logs = %q, did not want leak detection", logs)
	}

	logs = nil
	leak := openInternalHQStoreForTest(t,
		WithHQStoreSnapshotInterval(0),
		WithHQStoreMainTierBackstopTTL(time.Hour),
		WithHQStoreEphemeralBackstopTTL(time.Hour),
		WithHQStoreLeakDetector(1, events.NewFake()),
	)
	createClosedInternalHQBead(t, leak, Bead{Title: "leak-a"}, time.Now().Add(-2*time.Hour))
	createClosedInternalHQBead(t, leak, Bead{Title: "leak-b"}, time.Now().Add(-2*time.Hour))
	leak.runTTLSweep()

	if !joinedLogsContain(logs, "backstop leak detected") {
		t.Fatalf("leak reclaim logs = %q, want leak detection", logs)
	}
}

func openInternalHQStoreForTest(t *testing.T, opts ...HQStoreOption) *HQStore {
	t.Helper()
	store, err := OpenHQStore(t.TempDir(), opts...)
	if err != nil {
		t.Fatalf("OpenHQStore: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Shutdown(); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	return store
}

func createClosedInternalHQBead(t *testing.T, store *HQStore, b Bead, closedAt time.Time) Bead {
	t.Helper()
	created, err := store.Create(b)
	if err != nil {
		t.Fatalf("Create %q: %v", b.Title, err)
	}
	if err := store.Close(created.ID); err != nil {
		t.Fatalf("Close %q: %v", created.ID, err)
	}
	if err := store.SetMetadata(created.ID, hqClosedAtMetadataKey, closedAt.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("SetMetadata closed_at %q: %v", created.ID, err)
	}
	return created
}

func createRetainedInternalHQBead(t *testing.T, store *HQStore, b Bead, closedAt, deleteAfter time.Time) Bead {
	t.Helper()
	created, err := store.Create(b)
	if err != nil {
		t.Fatalf("Create %q: %v", b.Title, err)
	}
	if err := store.CloseWithRetention(created.ID, deleteAfter); err != nil {
		t.Fatalf("CloseWithRetention %q: %v", created.ID, err)
	}
	if err := store.SetMetadata(created.ID, hqClosedAtMetadataKey, closedAt.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("SetMetadata closed_at %q: %v", created.ID, err)
	}
	return created
}

func joinedLogsContain(logs []string, want string) bool {
	return strings.Contains(strings.Join(logs, "\n"), want)
}
