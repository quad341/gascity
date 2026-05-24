package beads

import (
	"time"
)

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
	now := time.Now()
	mainBefore := now.Add(-mainCutoff)
	ephemeralBefore := now.Add(-ephemeralCutoff)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureOpenLocked(); err != nil {
		return 0, err
	}

	ids := make([]string, 0)
	for id, bead := range s.main {
		if hqClosedBefore(bead, mainBefore) {
			ids = append(ids, id)
		}
	}
	for id, bead := range s.wisps {
		if hqClosedBefore(bead, ephemeralBefore) {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		s.deleteLocked(id)
	}
	return len(ids), nil
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
				_, _ = s.PurgeExpired()
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
