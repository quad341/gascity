package beads

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gastownhall/gascity/internal/events"
)

var hqStoreTTLLogf = log.Printf

type hqTTLSweepResult struct {
	Expired   int
	Retention int
	Backstop  hqBackstopReclaim
}

type hqBackstopReclaim struct {
	Main      int
	Ephemeral int
}

func (r hqBackstopReclaim) total() int {
	return r.Main + r.Ephemeral
}

// PurgeExpired removes ephemeral beads whose expires_at metadata is in the
// past. It returns the number of beads removed.
func (s *HQStore) PurgeExpired() (int, error) {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureOpenLocked(); err != nil {
		return 0, err
	}

	var ids []string
	for id, bead := range s.wisps {
		expiresAt, ok := hqBeadExpiresAt(bead)
		if ok && expiresAt.Before(now) {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		s.deleteLocked(id)
	}
	return len(ids), nil
}

// DrainRetentionQueue deletes closed beads whose post-close retention window
// has elapsed. It returns the number of beads deleted.
func (s *HQStore) DrainRetentionQueue() int {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0
	}

	kept := s.retentionQueue[:0]
	deleted := 0
	for _, entry := range s.retentionQueue {
		if now.Before(entry.deleteAfter) {
			kept = append(kept, entry)
			continue
		}
		bead, ok := s.findLocked(entry.id)
		if !ok || bead.Status != "closed" {
			continue
		}
		s.deleteLocked(entry.id)
		deleted++
	}
	s.retentionQueue = kept
	return deleted
}

// PurgeBackstop removes orphaned closed records whose close timestamp is older
// than the supplied tier cutoff. It scans main-tier and ephemeral records with
// their respective cutoffs and returns the total number deleted.
func (s *HQStore) PurgeBackstop(mainCutoff, ephemeralCutoff time.Duration) (int, error) {
	reclaim, err := s.purgeBackstop(mainCutoff, ephemeralCutoff)
	return reclaim.total(), err
}

func (s *HQStore) purgeBackstop(mainCutoff, ephemeralCutoff time.Duration) (hqBackstopReclaim, error) {
	now := time.Now()
	mainBefore := now.Add(-mainCutoff)
	ephemeralBefore := now.Add(-ephemeralCutoff)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureOpenLocked(); err != nil {
		return hqBackstopReclaim{}, err
	}

	type candidate struct {
		id        string
		ephemeral bool
	}

	candidates := make([]candidate, 0)
	for id, bead := range s.main {
		if hqClosedBefore(bead, mainBefore) {
			candidates = append(candidates, candidate{id: id})
		}
	}
	for id, bead := range s.wisps {
		if hqClosedBefore(bead, ephemeralBefore) {
			candidates = append(candidates, candidate{id: id, ephemeral: true})
		}
	}

	var reclaim hqBackstopReclaim
	for _, candidate := range candidates {
		s.deleteLocked(candidate.id)
		if candidate.ephemeral {
			reclaim.Ephemeral++
		} else {
			reclaim.Main++
		}
	}
	return reclaim, nil
}

func (s *HQStore) runTTLSweep() hqTTLSweepResult {
	var result hqTTLSweepResult

	expired, err := s.PurgeExpired()
	if err != nil {
		hqStoreTTLLogf("hqstore ttl sweep: purge expired: %v", err)
		return result
	}
	result.Expired = expired

	result.Retention = s.DrainRetentionQueue()

	backstop, err := s.purgeBackstop(s.mainTierBackstopTTL, s.ephemeralBackstopTTL)
	if err != nil {
		hqStoreTTLLogf("hqstore ttl sweep: purge backstop: %v", err)
		return result
	}
	result.Backstop = backstop
	s.reportBackstopReclaim(backstop)
	return result
}

func (s *HQStore) reportBackstopReclaim(reclaim hqBackstopReclaim) {
	total := reclaim.total()
	if total == 0 {
		return
	}
	if s.leakThreshold > 0 && total > s.leakThreshold {
		hqStoreTTLLogf("hqstore ttl sweep: backstop leak detected reclaimed=%d threshold=%d main=%d ephemeral=%d",
			total, s.leakThreshold, reclaim.Main, reclaim.Ephemeral)
		s.emitBackstopLeakDetected(reclaim)
		return
	}
	if s.leakThreshold > 0 {
		hqStoreTTLLogf("hqstore ttl sweep: backstop reclaimed=%d threshold=%d main=%d ephemeral=%d",
			total, s.leakThreshold, reclaim.Main, reclaim.Ephemeral)
		return
	}
	hqStoreTTLLogf("hqstore ttl sweep: backstop reclaimed=%d leak_detector=disabled main=%d ephemeral=%d",
		total, reclaim.Main, reclaim.Ephemeral)
}

func (s *HQStore) emitBackstopLeakDetected(reclaim hqBackstopReclaim) {
	if s.leakRecorder == nil {
		return
	}
	payload := events.StoreBackstopLeakDetectedPayload{
		MainCount:      reclaim.Main,
		EphemeralCount: reclaim.Ephemeral,
		TotalCount:     reclaim.total(),
		Threshold:      s.leakThreshold,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		hqStoreTTLLogf("hqstore ttl sweep: marshal backstop leak event: %v", err)
		return
	}
	s.leakRecorder.Record(events.Event{
		Type:    events.StoreBackstopLeakDetected,
		Actor:   "controller",
		Subject: s.dir,
		Payload: raw,
	})
}

func hqBeadExpiresAt(b Bead) (time.Time, bool) {
	if len(b.Metadata) == 0 {
		return time.Time{}, false
	}
	raw := b.Metadata[hqExpiresAtMetadataKey]
	if raw == "" {
		raw = b.Metadata[hqExpiresAtMetadataAlt]
	}
	if raw == "" {
		return time.Time{}, false
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false
	}
	return expiresAt, true
}

func hqClosedBefore(b Bead, cutoff time.Time) bool {
	if b.Status != "closed" {
		return false
	}
	closedAt, ok := hqBeadClosedAt(b)
	if !ok {
		return false
	}
	return closedAt.Before(cutoff) || closedAt.Equal(cutoff)
}

func hqHasClosedAt(b Bead) bool {
	_, ok := hqBeadClosedAt(b)
	return ok
}

func hqBeadClosedAt(b Bead) (time.Time, bool) {
	if len(b.Metadata) == 0 {
		return time.Time{}, false
	}
	raw := b.Metadata[hqClosedAtMetadataKey]
	if raw == "" {
		raw = b.Metadata[hqClosedAtMetadataAlt]
	}
	if raw == "" {
		return time.Time{}, false
	}
	closedAt, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false
	}
	return closedAt, true
}

func stampHQClosedAt(b *Bead, closedAt time.Time) {
	if b.Metadata == nil {
		b.Metadata = make(map[string]string, 1)
	}
	b.Metadata[hqClosedAtMetadataKey] = closedAt.Format(time.RFC3339Nano)
}

func (s *HQStore) startTTLSweeper() {
	if s.ttlInterval <= 0 {
		return
	}
	s.ttlStop = make(chan struct{})
	s.ttlDone = make(chan struct{})
	go func() {
		defer close(s.ttlDone)
		ticker := time.NewTicker(s.ttlInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.runTTLSweep()
			case <-s.ttlStop:
				return
			}
		}
	}()
}

func (s *HQStore) stopTTLSweeper() {
	if s.ttlStop == nil {
		return
	}
	close(s.ttlStop)
	<-s.ttlDone
	s.ttlStop = nil
	s.ttlDone = nil
}
