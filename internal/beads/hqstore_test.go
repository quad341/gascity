package beads_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/beads/beadstest"
	"github.com/gastownhall/gascity/internal/events"
)

func TestHQStoreConformance(t *testing.T) {
	factory := func() beads.Store {
		t.Helper()
		store, err := beads.OpenHQStore(t.TempDir())
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

	beadstest.RunStoreTests(t, factory)
	beadstest.RunDepTests(t, factory)
	beadstest.RunCreationOrderTests(t, factory)
}

func TestHQStoreRecoversFlushedSnapshotAfterSIGKILL(t *testing.T) {
	if os.Getenv("HQSTORE_SIGKILL_HELPER") == "1" {
		hqStoreSIGKILLHelper(t)
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestHQStoreRecoversFlushedSnapshotAfterSIGKILL")
	cmd.Env = append(os.Environ(),
		"HQSTORE_SIGKILL_HELPER=1",
		"HQSTORE_DIR="+dir,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting helper: %v", err)
	}

	// The helper writes the id file only AFTER Snapshot() returns, so its
	// presence guarantees a durable snapshot exists on disk.
	idPath := filepath.Join(dir, "created-id")
	var id string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(idPath)
		if err == nil {
			id = string(data)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if id == "" {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("helper did not write created bead id")
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("killing helper: %v", err)
	}
	_ = cmd.Wait()

	recovered, err := beads.OpenHQStore(dir)
	if err != nil {
		t.Fatalf("reopen after kill: %v", err)
	}
	t.Cleanup(func() {
		if err := recovered.Shutdown(); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	got, err := recovered.Get(id)
	if err != nil {
		t.Fatalf("Get(%q) after kill: %v", id, err)
	}
	if got.Title != "persist-before-sigkill" {
		t.Fatalf("recovered title = %q, want persist-before-sigkill", got.Title)
	}
}

func hqStoreSIGKILLHelper(t *testing.T) {
	t.Helper()
	dir := os.Getenv("HQSTORE_DIR")
	if dir == "" {
		t.Fatal("HQSTORE_DIR is required")
	}
	// Disable the periodic snapshotter so the only durable state is the one we
	// force via Snapshot() — this makes the test deterministic about what
	// survives the kill.
	store, err := beads.OpenHQStore(dir, beads.WithHQStoreSnapshotInterval(0))
	if err != nil {
		t.Fatalf("helper OpenHQStore: %v", err)
	}
	created, err := store.Create(beads.Bead{Title: "persist-before-sigkill"})
	if err != nil {
		t.Fatalf("helper Create: %v", err)
	}
	if err := store.Snapshot(); err != nil {
		t.Fatalf("helper Snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "created-id"), []byte(created.ID), 0o644); err != nil {
		t.Fatalf("helper write id: %v", err)
	}
	select {}
}

func TestHQStoreSnapshotRoundTripAcrossShutdown(t *testing.T) {
	dir := t.TempDir()
	store, err := beads.OpenHQStore(dir, beads.WithHQStoreSnapshotInterval(0))
	if err != nil {
		t.Fatalf("OpenHQStore: %v", err)
	}
	first, err := store.Create(beads.Bead{Title: "snapshotted", Metadata: map[string]string{"phase": "one"}})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, err := store.Create(beads.Bead{Title: "wisp", Type: "message", Ephemeral: true})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if err := store.DepAdd(first.ID, second.ID, "tracks"); err != nil {
		t.Fatalf("DepAdd: %v", err)
	}
	// Shutdown flushes a final snapshot even with the periodic snapshotter off.
	if err := store.Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	recovered, err := beads.OpenHQStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() {
		if err := recovered.Shutdown(); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	got, err := recovered.Get(first.ID)
	if err != nil {
		t.Fatalf("Get first: %v", err)
	}
	if got.Metadata["phase"] != "one" {
		t.Fatalf("metadata phase = %q, want one", got.Metadata["phase"])
	}
	// Ephemeral routing must survive the snapshot round-trip.
	wisp, err := recovered.Get(second.ID)
	if err != nil {
		t.Fatalf("Get second: %v", err)
	}
	if !wisp.Ephemeral {
		t.Fatalf("recovered wisp Ephemeral = false, want true")
	}
	deps, err := recovered.DepList(first.ID, "down")
	if err != nil {
		t.Fatalf("DepList: %v", err)
	}
	if len(deps) != 1 || deps[0].DependsOnID != second.ID {
		t.Fatalf("deps = %+v, want dependency on %s", deps, second.ID)
	}
}

func TestHQStorePeriodicSnapshotFlushes(t *testing.T) {
	dir := t.TempDir()
	store, err := beads.OpenHQStore(dir, beads.WithHQStoreSnapshotInterval(50*time.Millisecond))
	if err != nil {
		t.Fatalf("OpenHQStore: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Shutdown(); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	created, err := store.Create(beads.Bead{Title: "periodic"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Wait for the background snapshotter to publish a snapshot file.
	snapPath := filepath.Join(dir, "snapshot.jsonl.gz")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(snapPath); statErr == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, statErr := os.Stat(snapPath); statErr != nil {
		t.Fatalf("periodic snapshot not published: %v", statErr)
	}
	if err := store.LastSnapshotErr(); err != nil {
		t.Fatalf("background snapshot error: %v", err)
	}

	// A fresh open (without shutting down the first) should see the periodic
	// snapshot — reflecting what would survive a crash after the flush.
	recovered, err := beads.OpenHQStore(dir, beads.WithHQStoreSnapshotInterval(0))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() {
		if err := recovered.Shutdown(); err != nil {
			t.Errorf("Shutdown recovered: %v", err)
		}
	})
	if _, err := recovered.Get(created.ID); err != nil {
		t.Fatalf("Get(%q) from periodic snapshot: %v", created.ID, err)
	}
}

func TestHQStorePurgeExpired(t *testing.T) {
	store, err := beads.OpenHQStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenHQStore: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Shutdown(); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})

	expired, err := store.Create(beads.Bead{
		Title:     "expired",
		Type:      "order-tracking",
		Ephemeral: true,
		Metadata: map[string]string{
			"expires_at": time.Now().Add(-time.Second).Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		t.Fatalf("Create expired: %v", err)
	}
	live, err := store.Create(beads.Bead{
		Title:     "live",
		Type:      "order-tracking",
		Ephemeral: true,
		Metadata: map[string]string{
			"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		t.Fatalf("Create live: %v", err)
	}

	purged, err := store.PurgeExpired()
	if err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if purged != 1 {
		t.Fatalf("PurgeExpired purged %d, want 1", purged)
	}
	if _, err := store.Get(expired.ID); !errors.Is(err, beads.ErrNotFound) {
		t.Fatalf("Get expired error = %v, want ErrNotFound", err)
	}
	if _, err := store.Get(live.ID); err != nil {
		t.Fatalf("Get live: %v", err)
	}
}

func TestHQStoreCloseWithRetentionSetsStatusClosed(t *testing.T) {
	store, err := beads.OpenHQStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenHQStore: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Shutdown(); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})

	created, err := store.Create(beads.Bead{Title: "retained"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.CloseWithRetention(created.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CloseWithRetention: %v", err)
	}

	got, err := store.Get(created.ID)
	if err != nil {
		t.Fatalf("Get retained: %v", err)
	}
	if got.Status != "closed" {
		t.Fatalf("status = %q, want closed", got.Status)
	}
	if _, err := time.Parse(time.RFC3339Nano, got.Metadata["closed_at"]); err != nil {
		t.Fatalf("closed_at metadata parse: %v", err)
	}
}

func TestHQStoreRetentionQueueDrainsExpired(t *testing.T) {
	store, err := beads.OpenHQStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenHQStore: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Shutdown(); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})

	expiredA, err := store.Create(beads.Bead{Title: "expired-a"})
	if err != nil {
		t.Fatalf("Create expiredA: %v", err)
	}
	expiredB, err := store.Create(beads.Bead{Title: "expired-b"})
	if err != nil {
		t.Fatalf("Create expiredB: %v", err)
	}
	future, err := store.Create(beads.Bead{Title: "future"})
	if err != nil {
		t.Fatalf("Create future: %v", err)
	}

	if err := store.CloseWithRetention(expiredA.ID, time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("CloseWithRetention expiredA: %v", err)
	}
	if err := store.CloseWithRetention(future.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CloseWithRetention future: %v", err)
	}
	if err := store.CloseWithRetention(expiredB.ID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("CloseWithRetention expiredB: %v", err)
	}

	drained := store.DrainRetentionQueue()
	if drained != 2 {
		t.Fatalf("DrainRetentionQueue drained %d, want 2", drained)
	}
	for _, id := range []string{expiredA.ID, expiredB.ID} {
		if _, err := store.Get(id); !errors.Is(err, beads.ErrNotFound) {
			t.Fatalf("Get(%q) error = %v, want ErrNotFound", id, err)
		}
	}
	if _, err := store.Get(future.ID); err != nil {
		t.Fatalf("Get future: %v", err)
	}
}

func TestHQStorePurgeBackstopHonorsCutoffs(t *testing.T) {
	store, err := beads.OpenHQStore(t.TempDir(),
		beads.WithHQStoreMainTierBackstopTTL(24*time.Hour),
		beads.WithHQStoreEphemeralBackstopTTL(10*time.Minute),
		beads.WithHQStoreLeakDetector(5, events.Discard),
	)
	if err != nil {
		t.Fatalf("OpenHQStore: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Shutdown(); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})

	oldMain := createClosedHQBead(t, store, beads.Bead{Title: "old-main"}, time.Now().Add(-25*time.Hour))
	recentMain := createClosedHQBead(t, store, beads.Bead{Title: "recent-main"}, time.Now().Add(-23*time.Hour))
	oldWisp := createClosedHQBead(t, store, beads.Bead{Title: "old-wisp", Ephemeral: true}, time.Now().Add(-11*time.Minute))
	recentWisp := createClosedHQBead(t, store, beads.Bead{Title: "recent-wisp", Ephemeral: true}, time.Now().Add(-9*time.Minute))

	purged, err := store.PurgeBackstop(24*time.Hour, 10*time.Minute)
	if err != nil {
		t.Fatalf("PurgeBackstop: %v", err)
	}
	if purged != 2 {
		t.Fatalf("PurgeBackstop purged %d, want 2", purged)
	}
	for _, id := range []string{oldMain.ID, oldWisp.ID} {
		if _, err := store.Get(id); !errors.Is(err, beads.ErrNotFound) {
			t.Fatalf("Get(%q) error = %v, want ErrNotFound", id, err)
		}
	}
	for _, id := range []string{recentMain.ID, recentWisp.ID} {
		if _, err := store.Get(id); err != nil {
			t.Fatalf("Get(%q): %v", id, err)
		}
	}
}

func createClosedHQBead(t *testing.T, store *beads.HQStore, b beads.Bead, closedAt time.Time) beads.Bead {
	t.Helper()
	created, err := store.Create(b)
	if err != nil {
		t.Fatalf("Create %q: %v", b.Title, err)
	}
	if err := store.Close(created.ID); err != nil {
		t.Fatalf("Close %q: %v", created.ID, err)
	}
	if err := store.SetMetadata(created.ID, "closed_at", closedAt.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("SetMetadata closed_at %q: %v", created.ID, err)
	}
	return created
}

func TestHQStoreConcurrentCreateUpdate(t *testing.T) {
	// Run with the periodic snapshotter active so the race detector also
	// exercises concurrent ExportAll reads against the write path.
	store, err := beads.OpenHQStore(t.TempDir(), beads.WithHQStoreSnapshotInterval(20*time.Millisecond))
	if err != nil {
		t.Fatalf("OpenHQStore: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Shutdown(); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})

	const workers = 32
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := range workers {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			created, err := store.Create(beads.Bead{
				Title:    fmt.Sprintf("worker-%d", i),
				Assignee: "builder",
			})
			if err != nil {
				errs <- err
				return
			}
			status := "in_progress"
			if err := store.Update(created.ID, beads.UpdateOpts{Status: &status}); err != nil {
				errs <- err
				return
			}
			if err := store.SetMetadataBatch(created.ID, map[string]string{"worker": fmt.Sprint(i)}); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent worker error: %v", err)
		}
	}

	got, err := store.List(beads.ListQuery{Assignee: "builder", Status: "in_progress", AllowScan: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != workers {
		t.Fatalf("List returned %d workers, want %d", len(got), workers)
	}
	seen := make(map[string]bool, workers)
	for _, b := range got {
		if seen[b.ID] {
			t.Fatalf("duplicate ID %q", b.ID)
		}
		seen[b.ID] = true
	}
}
