package beads

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
	"unsafe"
)

func TestCachingStorePrimeLoadsOpenWispsAndServesTierWispsFromCache(t *testing.T) {
	backing := &wispsRecordingStore{Store: NewMemStore()}
	if _, err := backing.Create(Bead{Title: "issue", Labels: []string{"k"}}); err != nil {
		t.Fatalf("Create issue: %v", err)
	}
	openWisp, err := backing.Create(Bead{Title: "open wisp", Labels: []string{"k"}, Ephemeral: true})
	if err != nil {
		t.Fatalf("Create open wisp: %v", err)
	}
	closedWisp, err := backing.Create(Bead{Title: "closed wisp", Labels: []string{"k"}, Ephemeral: true})
	if err != nil {
		t.Fatalf("Create closed wisp: %v", err)
	}
	if err := backing.Close(closedWisp.ID); err != nil {
		t.Fatalf("Close closed wisp: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	assertCachedWisp(t, cache, openWisp.ID, "open wisp", "open")
	assertNoCachedWisp(t, cache, closedWisp.ID)

	direct := NewCachingStoreForTest(backing, nil)
	primeWispsForTest(t, direct)
	assertCachedWisp(t, direct, openWisp.ID, "open wisp", "open")

	backing.resetListCalls()
	updatedTitle := "backing changed after prime"
	if err := backing.Update(openWisp.ID, UpdateOpts{Title: &updatedTitle}); err != nil {
		t.Fatalf("Update backing: %v", err)
	}

	got, err := cache.List(ListQuery{Label: "k", TierMode: TierWisps, Sort: SortCreatedAsc})
	if err != nil {
		t.Fatalf("List(TierWisps): %v", err)
	}
	requireBeadIDs(t, got, openWisp.ID)
	if got[0].Title != "open wisp" {
		t.Fatalf("TierWisps title = %q, want cached title", got[0].Title)
	}
	if calls := backing.listCallsForTier(TierWisps); calls != 0 {
		t.Fatalf("TierWisps cache hit made %d backing List calls, want 0", calls)
	}
}

func TestCachingStoreTierWispsFallbacksBypassCache(t *testing.T) {
	t.Run("uninitialized", func(t *testing.T) {
		backing := &wispsRecordingStore{Store: NewMemStore()}
		wisp, err := backing.Create(Bead{Title: "wisp", Labels: []string{"k"}, Ephemeral: true})
		if err != nil {
			t.Fatalf("Create wisp: %v", err)
		}
		cache := NewCachingStoreForTest(backing, nil)

		got, err := cache.List(ListQuery{Label: "k", TierMode: TierWisps})
		if err != nil {
			t.Fatalf("List(TierWisps): %v", err)
		}
		requireBeadIDs(t, got, wisp.ID)
		if calls := backing.listCallsForTier(TierWisps); calls != 1 {
			t.Fatalf("backing TierWisps calls = %d, want 1", calls)
		}
	})

	t.Run("degraded", func(t *testing.T) {
		backing := &wispsRecordingStore{Store: NewMemStore()}
		wisp, err := backing.Create(Bead{Title: "wisp", Labels: []string{"k"}, Ephemeral: true})
		if err != nil {
			t.Fatalf("Create wisp: %v", err)
		}
		cache := NewCachingStoreForTest(backing, nil)
		primeWispsForTest(t, cache)
		cache.mu.Lock()
		setCacheField(t, cache, "wispsState", cacheDegraded)
		cache.mu.Unlock()
		backing.resetListCalls()

		got, err := cache.List(ListQuery{Label: "k", TierMode: TierWisps})
		if err != nil {
			t.Fatalf("List(TierWisps): %v", err)
		}
		requireBeadIDs(t, got, wisp.ID)
		if calls := backing.listCallsForTier(TierWisps); calls != 1 {
			t.Fatalf("backing TierWisps calls = %d, want 1", calls)
		}
	})

	t.Run("live query", func(t *testing.T) {
		backing := &wispsRecordingStore{Store: NewMemStore()}
		wisp, err := backing.Create(Bead{Title: "before", Labels: []string{"k"}, Ephemeral: true})
		if err != nil {
			t.Fatalf("Create wisp: %v", err)
		}
		cache := NewCachingStoreForTest(backing, nil)
		primeWispsForTest(t, cache)
		after := "after"
		if err := backing.Update(wisp.ID, UpdateOpts{Title: &after}); err != nil {
			t.Fatalf("Update backing: %v", err)
		}
		backing.resetListCalls()

		got, err := cache.List(ListQuery{Label: "k", TierMode: TierWisps, Live: true})
		if err != nil {
			t.Fatalf("List(TierWisps live): %v", err)
		}
		requireBeadIDs(t, got, wisp.ID)
		if got[0].Title != after {
			t.Fatalf("live TierWisps title = %q, want backing title %q", got[0].Title, after)
		}
		if calls := backing.listCallsForTier(TierWisps); calls != 1 {
			t.Fatalf("backing TierWisps calls = %d, want 1", calls)
		}
	})

	t.Run("closed query", func(t *testing.T) {
		backing := &wispsRecordingStore{Store: NewMemStore()}
		wisp, err := backing.Create(Bead{Title: "wisp", Labels: []string{"k"}, Ephemeral: true})
		if err != nil {
			t.Fatalf("Create wisp: %v", err)
		}
		cache := NewCachingStoreForTest(backing, nil)
		primeWispsForTest(t, cache)
		if err := backing.Close(wisp.ID); err != nil {
			t.Fatalf("Close backing: %v", err)
		}
		backing.resetListCalls()

		got, err := cache.List(ListQuery{Status: "closed", TierMode: TierWisps})
		if err != nil {
			t.Fatalf("List(TierWisps closed): %v", err)
		}
		requireBeadIDs(t, got, wisp.ID)
		if calls := backing.listCallsForTier(TierWisps); calls != 1 {
			t.Fatalf("backing TierWisps calls = %d, want 1", calls)
		}
	})
}

func TestCachingStoreTierBothUsesIndependentTierCacheFallbacks(t *testing.T) {
	t.Run("both live", func(t *testing.T) {
		backing := &wispsRecordingStore{Store: NewMemStore()}
		issue, err := backing.Create(Bead{Title: "issue", Labels: []string{"k"}})
		if err != nil {
			t.Fatalf("Create issue: %v", err)
		}
		wisp, err := backing.Create(Bead{Title: "wisp", Labels: []string{"k"}, Ephemeral: true})
		if err != nil {
			t.Fatalf("Create wisp: %v", err)
		}
		cache := NewCachingStoreForTest(backing, nil)
		if err := cache.Prime(context.Background()); err != nil {
			t.Fatalf("Prime: %v", err)
		}
		changed := "changed in backing"
		if err := backing.Update(issue.ID, UpdateOpts{Title: &changed}); err != nil {
			t.Fatalf("Update issue backing: %v", err)
		}
		if err := backing.Update(wisp.ID, UpdateOpts{Title: &changed}); err != nil {
			t.Fatalf("Update wisp backing: %v", err)
		}
		backing.resetListCalls()

		got, err := cache.List(ListQuery{Label: "k", TierMode: TierBoth, Sort: SortCreatedAsc})
		if err != nil {
			t.Fatalf("List(TierBoth): %v", err)
		}
		requireBeadIDs(t, got, issue.ID, wisp.ID)
		for _, bead := range got {
			if bead.Title == changed {
				t.Fatalf("List(TierBoth) returned backing-mutated bead %+v, want cached snapshot", bead)
			}
		}
		if backing.listCalls != 0 {
			t.Fatalf("TierBoth both-live cache hit made %d backing List calls, want 0", backing.listCalls)
		}
	})

	t.Run("issues live wisps uninitialized", func(t *testing.T) {
		backing := &wispsRecordingStore{Store: NewMemStore()}
		issue, err := backing.Create(Bead{Title: "issue", Labels: []string{"k"}})
		if err != nil {
			t.Fatalf("Create issue: %v", err)
		}
		wisp, err := backing.Create(Bead{Title: "wisp", Labels: []string{"k"}, Ephemeral: true})
		if err != nil {
			t.Fatalf("Create wisp: %v", err)
		}
		cache := NewCachingStoreForTest(backing, nil)
		if err := cache.Prime(context.Background()); err != nil {
			t.Fatalf("Prime: %v", err)
		}
		cache.mu.Lock()
		setCacheField(t, cache, "wisps", make(map[string]Bead))
		setCacheField(t, cache, "wispsState", cacheUninitialized)
		setCacheField(t, cache, "wispsLastFreshAt", time.Time{})
		cache.mu.Unlock()

		issueChanged := "issue changed in backing"
		wispChanged := "wisp changed in backing"
		if err := backing.Update(issue.ID, UpdateOpts{Title: &issueChanged}); err != nil {
			t.Fatalf("Update issue backing: %v", err)
		}
		if err := backing.Update(wisp.ID, UpdateOpts{Title: &wispChanged}); err != nil {
			t.Fatalf("Update wisp backing: %v", err)
		}
		backing.resetListCalls()

		got, err := cache.List(ListQuery{Label: "k", TierMode: TierBoth, Sort: SortCreatedAsc})
		if err != nil {
			t.Fatalf("List(TierBoth): %v", err)
		}
		requireBeadIDs(t, got, issue.ID, wisp.ID)
		if gotTitle(t, got, issue.ID) != "issue" {
			t.Fatalf("issue title = %q, want cached title", gotTitle(t, got, issue.ID))
		}
		if gotTitle(t, got, wisp.ID) != wispChanged {
			t.Fatalf("wisp title = %q, want backing fallback title %q", gotTitle(t, got, wisp.ID), wispChanged)
		}
		if calls := backing.listCallsForTier(TierWisps); calls != 1 {
			t.Fatalf("backing TierWisps calls = %d, want 1", calls)
		}
		if calls := backing.listCallsForTier(TierBoth); calls != 0 {
			t.Fatalf("backing TierBoth calls = %d, want 0 independent tier fallback", calls)
		}
		if calls := backing.listCallsForTier(TierIssues); calls != 0 {
			t.Fatalf("backing TierIssues calls = %d, want 0 for cacheable issues tier", calls)
		}
	})

	t.Run("both uninitialized", func(t *testing.T) {
		backing := &wispsRecordingStore{Store: NewMemStore()}
		issue, err := backing.Create(Bead{Title: "issue", Labels: []string{"k"}})
		if err != nil {
			t.Fatalf("Create issue: %v", err)
		}
		wisp, err := backing.Create(Bead{Title: "wisp", Labels: []string{"k"}, Ephemeral: true})
		if err != nil {
			t.Fatalf("Create wisp: %v", err)
		}
		cache := NewCachingStoreForTest(backing, nil)

		got, err := cache.List(ListQuery{Label: "k", TierMode: TierBoth, Sort: SortCreatedAsc})
		if err != nil {
			t.Fatalf("List(TierBoth): %v", err)
		}
		requireBeadIDs(t, got, issue.ID, wisp.ID)
		if calls := backing.listCallsForTier(TierBoth); calls != 1 {
			t.Fatalf("backing TierBoth calls = %d, want 1 full fallback", calls)
		}
	})
}

func TestCachingStoreCachedListSupportsWispsAndBothTiers(t *testing.T) {
	backing := &wispsRecordingStore{Store: NewMemStore()}
	issue, err := backing.Create(Bead{Title: "issue", Labels: []string{"k"}})
	if err != nil {
		t.Fatalf("Create issue: %v", err)
	}
	wisp, err := backing.Create(Bead{Title: "wisp", Labels: []string{"k"}, Ephemeral: true})
	if err != nil {
		t.Fatalf("Create wisp: %v", err)
	}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.resetListCalls()

	wisps, ok := cache.CachedList(ListQuery{Label: "k", TierMode: TierWisps})
	if !ok {
		t.Fatal("CachedList(TierWisps) ok=false, want true")
	}
	requireBeadIDs(t, wisps, wisp.ID)

	both, ok := cache.CachedList(ListQuery{Label: "k", TierMode: TierBoth, Sort: SortCreatedAsc})
	if !ok {
		t.Fatal("CachedList(TierBoth) ok=false, want true")
	}
	requireBeadIDs(t, both, issue.ID, wisp.ID)
	if backing.listCalls != 0 {
		t.Fatalf("CachedList made %d backing List calls, want 0", backing.listCalls)
	}

	closed, ok := cache.CachedList(ListQuery{Status: "closed", TierMode: TierWisps})
	if ok {
		t.Fatalf("CachedList(TierWisps closed) ok=true rows=%+v, want ok=false", closed)
	}
}

func TestCachingStoreRepeatedWispsReadsStayInCache(t *testing.T) {
	backing := &wispsRecordingStore{Store: NewMemStore()}
	issue, err := backing.Create(Bead{Title: "issue mail", Labels: []string{"mail-check"}})
	if err != nil {
		t.Fatalf("Create issue: %v", err)
	}
	wisp, err := backing.Create(Bead{Title: "wisp mail", Labels: []string{"mail-check"}, Ephemeral: true})
	if err != nil {
		t.Fatalf("Create wisp: %v", err)
	}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.resetListCalls()

	for i := 0; i < 25; i++ {
		wisps, err := cache.List(ListQuery{Label: "mail-check", TierMode: TierWisps})
		if err != nil {
			t.Fatalf("List TierWisps pass %d: %v", i, err)
		}
		requireBeadIDs(t, wisps, wisp.ID)

		both, err := cache.List(ListQuery{Label: "mail-check", TierMode: TierBoth, Sort: SortCreatedAsc})
		if err != nil {
			t.Fatalf("List TierBoth pass %d: %v", i, err)
		}
		requireBeadIDs(t, both, issue.ID, wisp.ID)
	}

	if backing.listCalls != 0 {
		t.Fatalf("repeated cached wisps reads made %d backing List calls, want 0", backing.listCalls)
	}
}

func TestCachingStoreEphemeralWritesUpdateWispsCache(t *testing.T) {
	backing := NewMemStore()
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}

	created, err := cache.Create(Bead{Title: "created", Ephemeral: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	assertCachedWisp(t, cache, created.ID, "created", "open")
	assertNoCachedIssue(t, cache, created.ID)

	updatedTitle := "updated"
	if err := cache.Update(created.ID, UpdateOpts{Title: &updatedTitle}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	assertCachedWisp(t, cache, created.ID, updatedTitle, "open")

	if err := cache.Close(created.ID); err != nil {
		t.Fatalf("Close: %v", err)
	}
	assertCachedWisp(t, cache, created.ID, updatedTitle, "closed")

	if err := cache.Reopen(created.ID); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	assertCachedWisp(t, cache, created.ID, updatedTitle, "open")

	if err := cache.Delete(created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	assertNoCachedWisp(t, cache, created.ID)
}

func TestCachingStoreApplyEventUpdatesWispsCache(t *testing.T) {
	backing := NewMemStore()
	cache := NewCachingStoreForTest(backing, nil)
	primeWispsForTest(t, cache)

	created, err := backing.Create(Bead{Title: "created", Ephemeral: true})
	if err != nil {
		t.Fatalf("Create backing wisp: %v", err)
	}
	createdPayload, err := json.Marshal(created)
	if err != nil {
		t.Fatalf("Marshal created: %v", err)
	}
	cache.ApplyEvent("bead.created", createdPayload)
	assertCachedWisp(t, cache, created.ID, "created", "open")

	cache.ApplyEvent("bead.updated", json.RawMessage(`{"id":"`+created.ID+`","title":"updated"}`))
	assertCachedWisp(t, cache, created.ID, "updated", "open")

	cache.ApplyEvent("bead.closed", json.RawMessage(`{"id":"`+created.ID+`","status":"closed"}`))
	assertCachedWisp(t, cache, created.ID, "updated", "closed")
}

func TestCachingStoreRunWispsReconciliationReplacesWispsCache(t *testing.T) {
	backing := NewMemStore()
	stale, err := backing.Create(Bead{Title: "stale", Ephemeral: true})
	if err != nil {
		t.Fatalf("Create stale: %v", err)
	}
	cache := NewCachingStoreForTest(backing, nil)
	primeWispsForTest(t, cache)
	if err := backing.Close(stale.ID); err != nil {
		t.Fatalf("Close stale backing: %v", err)
	}
	fresh, err := backing.Create(Bead{Title: "fresh", Ephemeral: true})
	if err != nil {
		t.Fatalf("Create fresh: %v", err)
	}

	runWispsReconciliationForTest(t, cache)

	assertNoCachedWisp(t, cache, stale.ID)
	assertCachedWisp(t, cache, fresh.ID, "fresh", "open")
}

func TestCachingStoreWispsReconciliationFailureAndRecovery(t *testing.T) {
	backing := &wispsRecordingStore{Store: NewMemStore()}
	cache := NewCachingStoreForTest(backing, nil)
	cache.mu.Lock()
	setCacheField(t, cache, "wispsState", cacheLive)
	cache.mu.Unlock()

	backing.listErr = func(query ListQuery) error {
		if query.TierMode == TierWisps {
			return errors.New("wisps unavailable")
		}
		return nil
	}
	for i := 0; i < maxCacheSyncFailures; i++ {
		runWispsReconciliationForTest(t, cache)
	}
	stats := cache.Stats()
	if got := cacheStatsInt(t, stats, "WispsSyncFailures"); got != maxCacheSyncFailures {
		t.Fatalf("WispsSyncFailures = %d, want %d", got, maxCacheSyncFailures)
	}
	if got := cacheStatsString(t, stats, "WispsState"); got != "degraded" {
		t.Fatalf("WispsState = %q, want degraded", got)
	}
	if cacheStatsTime(t, stats, "WispsLastProblemAt").IsZero() {
		t.Fatal("WispsLastProblemAt is zero after repeated reconcile failures")
	}
	if delay := nextWispsReconcileDelayForTest(t, cache, time.Now()); delay <= 0 {
		t.Fatalf("nextWispsReconcileDelay after degradation = %s, want positive backoff", delay)
	}

	wisp, err := backing.Create(Bead{Title: "recovered", Ephemeral: true})
	if err != nil {
		t.Fatalf("Create recovered wisp: %v", err)
	}
	backing.listErr = nil
	runWispsReconciliationForTest(t, cache)

	stats = cache.Stats()
	if got := cacheStatsInt(t, stats, "WispsSyncFailures"); got != 0 {
		t.Fatalf("WispsSyncFailures after recovery = %d, want 0", got)
	}
	if got := cacheStatsString(t, stats, "WispsState"); got != "live" {
		t.Fatalf("WispsState after recovery = %q, want live", got)
	}
	assertCachedWisp(t, cache, wisp.ID, "recovered", "open")
}

func TestCachingStoreNextWispsReconcileDelay(t *testing.T) {
	cache := NewCachingStoreForTest(NewMemStore(), nil)
	now := time.Now()

	if delay := nextWispsReconcileDelayForTest(t, cache, now); delay != 0 {
		t.Fatalf("uninitialized delay = %s, want immediate", delay)
	}

	cache.mu.Lock()
	setCacheField(t, cache, "wispsState", cacheLive)
	setCacheField(t, cache, "wispsLastFreshAt", now)
	cache.mu.Unlock()
	if delay := nextWispsReconcileDelayForTest(t, cache, now); delay != cacheWispsReconcileInterval {
		t.Fatalf("fresh live delay = %s, want %s", delay, cacheWispsReconcileInterval)
	}
	if delay := nextWispsReconcileDelayForTest(t, cache, now.Add(cacheWispsReconcileInterval)); delay != 0 {
		t.Fatalf("due live delay = %s, want immediate", delay)
	}

	cache.mu.Lock()
	setCacheField(t, cache, "wispsState", cacheDegraded)
	cache.stats.WispsLastProblemAt = now
	cache.mu.Unlock()
	if delay := nextWispsReconcileDelayForTest(t, cache, now); delay != cacheReconcileFailureBackoff {
		t.Fatalf("degraded backoff delay = %s, want %s", delay, cacheReconcileFailureBackoff)
	}
	if delay := nextWispsReconcileDelayForTest(t, cache, now.Add(cacheReconcileFailureBackoff)); delay != 0 {
		t.Fatalf("expired degraded delay = %s, want immediate", delay)
	}
}

func TestCachingStoreStatsExposeWispsState(t *testing.T) {
	backing := NewMemStore()
	if _, err := backing.Create(Bead{Title: "wisp", Ephemeral: true}); err != nil {
		t.Fatalf("Create wisp: %v", err)
	}
	cache := NewCachingStoreForTest(backing, nil)
	primeWispsForTest(t, cache)

	stats := cache.Stats()
	if got := cacheStatsString(t, stats, "WispsState"); got != "live" {
		t.Fatalf("WispsState = %q, want live", got)
	}
	if got := cacheStatsInt(t, stats, "WispsBeadCount"); got != 1 {
		t.Fatalf("WispsBeadCount = %d, want 1", got)
	}
	if got := cacheStatsInt(t, stats, "WispsSyncFailures"); got != 0 {
		t.Fatalf("WispsSyncFailures = %d, want 0", got)
	}
	if cacheStatsTime(t, stats, "WispsLastFreshAt").IsZero() {
		t.Fatal("WispsLastFreshAt is zero")
	}
}

type cachingStoreWispsPrimer interface {
	PrimeWisps() error
}

func primeWispsForTest(t *testing.T, cache *CachingStore) {
	t.Helper()
	primer, ok := any(cache).(cachingStoreWispsPrimer)
	if !ok {
		t.Fatal("CachingStore is missing PrimeWisps()")
	}
	if err := primer.PrimeWisps(); err != nil {
		t.Fatalf("PrimeWisps: %v", err)
	}
}

type cachingStoreWispsReconciler interface {
	runWispsReconciliation()
}

func runWispsReconciliationForTest(t *testing.T, cache *CachingStore) {
	t.Helper()
	reconciler, ok := any(cache).(cachingStoreWispsReconciler)
	if !ok {
		t.Fatal("CachingStore is missing runWispsReconciliation()")
	}
	reconciler.runWispsReconciliation()
}

type cachingStoreWispsDelay interface {
	nextWispsReconcileDelay(time.Time) time.Duration
}

func nextWispsReconcileDelayForTest(t *testing.T, cache *CachingStore, now time.Time) time.Duration {
	t.Helper()
	delayer, ok := any(cache).(cachingStoreWispsDelay)
	if !ok {
		t.Fatal("CachingStore is missing nextWispsReconcileDelay(time.Time)")
	}
	return delayer.nextWispsReconcileDelay(now)
}

type wispsRecordingStore struct {
	Store
	listCalls   int
	listQueries []ListQuery
	listErr     func(ListQuery) error
}

func (s *wispsRecordingStore) List(query ListQuery) ([]Bead, error) {
	s.listCalls++
	s.listQueries = append(s.listQueries, query)
	if s.listErr != nil {
		if err := s.listErr(query); err != nil {
			return nil, err
		}
	}
	return s.Store.List(query)
}

func (s *wispsRecordingStore) resetListCalls() {
	s.listCalls = 0
	s.listQueries = nil
}

func (s *wispsRecordingStore) listCallsForTier(tier TierMode) int {
	var count int
	for _, query := range s.listQueries {
		if query.TierMode == tier {
			count++
		}
	}
	return count
}

func assertCachedWisp(t *testing.T, cache *CachingStore, id, title, status string) {
	t.Helper()
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	got, ok := cachedWispByID(t, cache, id)
	if !ok {
		t.Fatalf("wisp %s not cached in wisps map", id)
	}
	if got.Title != title || got.Status != status || !got.Ephemeral {
		t.Fatalf("cached wisp = %+v, want title=%q status=%q ephemeral=true", got, title, status)
	}
}

func assertNoCachedWisp(t *testing.T, cache *CachingStore, id string) {
	t.Helper()
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	if got, ok := cachedWispByID(t, cache, id); ok {
		t.Fatalf("wisp %s cached unexpectedly: %+v", id, got)
	}
}

func cachedWispByID(t *testing.T, cache *CachingStore, id string) (Bead, bool) {
	t.Helper()
	field := cacheField(t, cache, "wisps")
	if field.Kind() != reflect.Map || field.Type().Key().Kind() != reflect.String || field.Type().Elem() != reflect.TypeOf(Bead{}) {
		t.Fatalf("CachingStore.wisps has type %s, want map[string]Bead", field.Type())
	}
	got := field.MapIndex(reflect.ValueOf(id))
	if !got.IsValid() {
		return Bead{}, false
	}
	return got.Interface().(Bead), true
}

func assertNoCachedIssue(t *testing.T, cache *CachingStore, id string) {
	t.Helper()
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	if got, ok := cache.beads[id]; ok {
		t.Fatalf("ephemeral bead %s cached in issues map: %+v", id, got)
	}
}

func requireBeadIDs(t *testing.T, got []Bead, want ...string) {
	t.Helper()
	ids := make(map[string]int, len(got))
	for _, bead := range got {
		ids[bead.ID]++
	}
	if len(got) != len(want) {
		t.Fatalf("got ids %v, want %v", beadIDCounts(ids), want)
	}
	for _, id := range want {
		if ids[id] != 1 {
			t.Fatalf("got ids %v, want %v", beadIDCounts(ids), want)
		}
	}
}

func gotTitle(t *testing.T, got []Bead, id string) string {
	t.Helper()
	for _, bead := range got {
		if bead.ID == id {
			return bead.Title
		}
	}
	t.Fatalf("missing bead %s in %+v", id, got)
	return ""
}

func beadIDCounts(ids map[string]int) map[string]int {
	out := make(map[string]int, len(ids))
	for id, count := range ids {
		out[id] = count
	}
	return out
}

func setCacheField(t *testing.T, cache *CachingStore, name string, value any) {
	t.Helper()
	field := cacheField(t, cache, name)
	next := reflect.ValueOf(value)
	if !next.Type().AssignableTo(field.Type()) {
		t.Fatalf("cannot assign %s to CachingStore.%s (%s)", next.Type(), name, field.Type())
	}
	field.Set(next)
}

func cacheField(t *testing.T, cache *CachingStore, name string) reflect.Value {
	t.Helper()
	field := reflect.ValueOf(cache).Elem().FieldByName(name)
	if !field.IsValid() {
		t.Fatalf("CachingStore is missing field %s", name)
	}
	if !field.CanAddr() {
		t.Fatalf("CachingStore.%s is not addressable", name)
	}
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
}

func cacheStatsInt(t *testing.T, stats CacheStats, name string) int {
	t.Helper()
	field := cacheStatsField(t, stats, name)
	switch field.Kind() {
	case reflect.Int:
		return int(field.Int())
	default:
		t.Fatalf("CacheStats.%s has kind %s, want int", name, field.Kind())
		return 0
	}
}

func cacheStatsString(t *testing.T, stats CacheStats, name string) string {
	t.Helper()
	field := cacheStatsField(t, stats, name)
	if field.Kind() != reflect.String {
		t.Fatalf("CacheStats.%s has kind %s, want string", name, field.Kind())
	}
	return field.String()
}

func cacheStatsTime(t *testing.T, stats CacheStats, name string) time.Time {
	t.Helper()
	field := cacheStatsField(t, stats, name)
	if field.Type() != reflect.TypeOf(time.Time{}) {
		t.Fatalf("CacheStats.%s has type %s, want time.Time", name, field.Type())
	}
	return field.Interface().(time.Time)
}

func cacheStatsField(t *testing.T, stats CacheStats, name string) reflect.Value {
	t.Helper()
	field := reflect.ValueOf(stats).FieldByName(name)
	if !field.IsValid() {
		t.Fatalf("CacheStats is missing field %s", name)
	}
	return field
}
