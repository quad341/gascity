package beads

import (
	"context"
	"errors"
	"testing"
)

// countingBackingStore wraps a Store and counts SetMetadata /
// SetMetadataBatch / Update / Close invocations so tests can assert when
// CachingStore short-circuits a no-op write before the backing call.
type countingBackingStore struct {
	Store
	setMetadataCalls      int
	setMetadataBatchCalls int
	updateCalls           int
	closeCalls            int
}

func (c *countingBackingStore) SetMetadata(id, key, value string) error {
	c.setMetadataCalls++
	return c.Store.SetMetadata(id, key, value)
}

func (c *countingBackingStore) SetMetadataBatch(id string, kvs map[string]string) error {
	c.setMetadataBatchCalls++
	return c.Store.SetMetadataBatch(id, kvs)
}

func (c *countingBackingStore) Update(id string, opts UpdateOpts) error {
	c.updateCalls++
	return c.Store.Update(id, opts)
}

func (c *countingBackingStore) Close(id string) error {
	c.closeCalls++
	return c.Store.Close(id)
}

type localMetadataBackingStore struct {
	Store
	setLocalStringCalls int
	getLocalStringCalls int
	setLocalStringErr   error
	getLocalStringErr   error
	local               map[string]map[string]string
}

func (s *localMetadataBackingStore) SetLocalString(beadID, key, value string) error {
	s.setLocalStringCalls++
	if s.setLocalStringErr != nil {
		return s.setLocalStringErr
	}
	if s.local == nil {
		s.local = make(map[string]map[string]string)
	}
	if s.local[beadID] == nil {
		s.local[beadID] = make(map[string]string)
	}
	s.local[beadID][key] = value
	return nil
}

func (s *localMetadataBackingStore) GetLocalString(beadID, key string) (string, bool, error) {
	s.getLocalStringCalls++
	if s.getLocalStringErr != nil {
		return "", false, s.getLocalStringErr
	}
	if s.local == nil || s.local[beadID] == nil {
		return "", false, nil
	}
	value, ok := s.local[beadID][key]
	return value, ok, nil
}

func requireCachedLocalString(t *testing.T, cache *CachingStore, beadID, key, want string) {
	t.Helper()
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	values, ok := cache.localMeta[beadID]
	if !ok {
		t.Fatalf("localMeta[%q] missing", beadID)
	}
	got, ok := values[key]
	if !ok {
		t.Fatalf("localMeta[%q][%q] missing", beadID, key)
	}
	if got != want {
		t.Fatalf("localMeta[%q][%q] = %q, want %q", beadID, key, got, want)
	}
}

func requireNoCachedLocalMeta(t *testing.T, cache *CachingStore, beadID string) {
	t.Helper()
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	if values, ok := cache.localMeta[beadID]; ok {
		t.Fatalf("localMeta[%q] = %#v, want absent", beadID, values)
	}
}

type txObservingBackingStore struct {
	Store
	txCalls   int
	commitMsg string
	afterFn   error
	onTxStart func()
}

func (s *txObservingBackingStore) Tx(commitMsg string, fn func(Tx) error) error {
	s.txCalls++
	s.commitMsg = commitMsg
	if s.onTxStart != nil {
		s.onTxStart()
	}
	if err := fn(s.Store); err != nil {
		return err
	}
	return s.afterFn
}

func TestCachingStoreSetLocalStringCachesAfterBackingSuccess(t *testing.T) {
	backing := &localMetadataBackingStore{Store: NewMemStore()}
	cache := NewCachingStoreForTest(backing, nil)

	if err := cache.SetLocalString("bd-1", "synced_at", "2026-05-17T22:00:00Z"); err != nil {
		t.Fatalf("SetLocalString: %v", err)
	}

	if backing.setLocalStringCalls != 1 {
		t.Fatalf("backing SetLocalString calls = %d, want 1", backing.setLocalStringCalls)
	}
	if backing.local["bd-1"]["synced_at"] != "2026-05-17T22:00:00Z" {
		t.Fatalf("backing local value = %q, want timestamp", backing.local["bd-1"]["synced_at"])
	}
	requireCachedLocalString(t, cache, "bd-1", "synced_at", "2026-05-17T22:00:00Z")
}

func TestCachingStoreSetLocalStringDoesNotCacheBackingErrors(t *testing.T) {
	wantErr := errors.New("local write failed")
	backing := &localMetadataBackingStore{Store: NewMemStore(), setLocalStringErr: wantErr}
	cache := NewCachingStoreForTest(backing, nil)

	if err := cache.SetLocalString("bd-1", "synced_at", "value"); !errors.Is(err, wantErr) {
		t.Fatalf("SetLocalString error = %v, want %v", err, wantErr)
	}

	requireNoCachedLocalMeta(t, cache, "bd-1")
}

func TestCachingStoreSetLocalStringDoesNotCacheUnsupportedBacking(t *testing.T) {
	cache := NewCachingStoreForTest(NewMemStore(), nil)

	if err := cache.SetLocalString("bd-1", "synced_at", "value"); !errors.Is(err, ErrLocalMetadataNotSupported) {
		t.Fatalf("SetLocalString error = %v, want ErrLocalMetadataNotSupported", err)
	}

	requireNoCachedLocalMeta(t, cache, "bd-1")
}

func TestCachingStoreGetLocalStringServesCacheHitWithoutBackingRead(t *testing.T) {
	backing := &localMetadataBackingStore{Store: NewMemStore()}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.SetLocalString("bd-1", "last_woke_at", "cached"); err != nil {
		t.Fatalf("SetLocalString: %v", err)
	}
	backing.local["bd-1"]["last_woke_at"] = "backing"

	value, ok, err := cache.GetLocalString("bd-1", "last_woke_at")
	if err != nil {
		t.Fatalf("GetLocalString: %v", err)
	}
	if !ok {
		t.Fatal("GetLocalString ok = false, want true")
	}
	if value != "cached" {
		t.Fatalf("GetLocalString value = %q, want cached", value)
	}
	if backing.getLocalStringCalls != 0 {
		t.Fatalf("backing GetLocalString calls = %d, want 0", backing.getLocalStringCalls)
	}
}

func TestCachingStoreGetLocalStringPopulatesCacheAfterBackingRead(t *testing.T) {
	backing := &localMetadataBackingStore{
		Store: NewMemStore(),
		local: map[string]map[string]string{
			"bd-1": {"pending_create_claim": "claim-1"},
		},
	}
	cache := NewCachingStoreForTest(backing, nil)

	value, ok, err := cache.GetLocalString("bd-1", "pending_create_claim")
	if err != nil {
		t.Fatalf("GetLocalString: %v", err)
	}
	if !ok {
		t.Fatal("GetLocalString ok = false, want true")
	}
	if value != "claim-1" {
		t.Fatalf("GetLocalString value = %q, want claim-1", value)
	}
	requireCachedLocalString(t, cache, "bd-1", "pending_create_claim", "claim-1")

	backing.local["bd-1"]["pending_create_claim"] = "claim-2"
	value, ok, err = cache.GetLocalString("bd-1", "pending_create_claim")
	if err != nil {
		t.Fatalf("second GetLocalString: %v", err)
	}
	if !ok || value != "claim-1" {
		t.Fatalf("second GetLocalString = %q, %v; want claim-1, true", value, ok)
	}
	if backing.getLocalStringCalls != 1 {
		t.Fatalf("backing GetLocalString calls = %d, want 1", backing.getLocalStringCalls)
	}
}

func TestCachingStoreGetLocalStringDoesNotCacheBackingErrors(t *testing.T) {
	wantErr := errors.New("local read failed")
	backing := &localMetadataBackingStore{Store: NewMemStore(), getLocalStringErr: wantErr}
	cache := NewCachingStoreForTest(backing, nil)

	if _, _, err := cache.GetLocalString("bd-1", "synced_at"); !errors.Is(err, wantErr) {
		t.Fatalf("GetLocalString error = %v, want %v", err, wantErr)
	}

	requireNoCachedLocalMeta(t, cache, "bd-1")
}

func TestCachingStoreDeleteEvictsLocalMetadata(t *testing.T) {
	backing := &localMetadataBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "local metadata"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	if err := cache.SetLocalString(bead.ID, "synced_at", "value"); err != nil {
		t.Fatalf("SetLocalString: %v", err)
	}

	if err := cache.Delete(bead.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	requireNoCachedLocalMeta(t, cache, bead.ID)
}

func TestCachingStoreTxEvictsLocalMetadataForTouchedBeads(t *testing.T) {
	backing := &localMetadataBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "local metadata", Metadata: map[string]string{"phase": "old"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	if err := cache.SetLocalString(bead.ID, "synced_at", "value"); err != nil {
		t.Fatalf("SetLocalString: %v", err)
	}

	if err := cache.Tx("local metadata tx", func(tx Tx) error {
		return tx.SetMetadataBatch(bead.ID, map[string]string{"phase": "new"})
	}); err != nil {
		t.Fatalf("Tx: %v", err)
	}

	requireNoCachedLocalMeta(t, cache, bead.ID)
}

func TestCachingStorePrimeEvictsLocalMetadataForDroppedBeads(t *testing.T) {
	backing := &localMetadataBackingStore{Store: NewMemStore()}
	dropped, err := backing.Create(Bead{Title: "dropped"})
	if err != nil {
		t.Fatalf("Create dropped: %v", err)
	}
	kept, err := backing.Create(Bead{Title: "kept"})
	if err != nil {
		t.Fatalf("Create kept: %v", err)
	}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	if err := cache.SetLocalString(dropped.ID, "synced_at", "dropped"); err != nil {
		t.Fatalf("SetLocalString dropped: %v", err)
	}
	if err := cache.SetLocalString(kept.ID, "synced_at", "kept"); err != nil {
		t.Fatalf("SetLocalString kept: %v", err)
	}
	if err := backing.Delete(dropped.ID); err != nil {
		t.Fatalf("backing Delete: %v", err)
	}

	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime after delete: %v", err)
	}

	requireNoCachedLocalMeta(t, cache, dropped.ID)
	requireCachedLocalString(t, cache, kept.ID, "synced_at", "kept")
}

func TestCachingStoreRunReconciliationEvictsLocalMetadataForDroppedBeads(t *testing.T) {
	backing := &localMetadataBackingStore{Store: NewMemStore()}
	dropped, err := backing.Create(Bead{Title: "dropped"})
	if err != nil {
		t.Fatalf("Create dropped: %v", err)
	}
	kept, err := backing.Create(Bead{Title: "kept"})
	if err != nil {
		t.Fatalf("Create kept: %v", err)
	}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	if err := cache.SetLocalString(dropped.ID, "synced_at", "dropped"); err != nil {
		t.Fatalf("SetLocalString dropped: %v", err)
	}
	if err := cache.SetLocalString(kept.ID, "synced_at", "kept"); err != nil {
		t.Fatalf("SetLocalString kept: %v", err)
	}
	if err := backing.Delete(dropped.ID); err != nil {
		t.Fatalf("backing Delete: %v", err)
	}

	cache.runReconciliation()

	requireNoCachedLocalMeta(t, cache, dropped.ID)
	requireCachedLocalString(t, cache, kept.ID, "synced_at", "kept")
}

func TestCachingStoreParentRefreshEvictsLocalMetadataForRemovedChild(t *testing.T) {
	backing := &localMetadataBackingStore{Store: NewMemStore()}
	parent, err := backing.Create(Bead{Title: "parent"})
	if err != nil {
		t.Fatalf("Create parent: %v", err)
	}
	child, err := backing.Create(Bead{Title: "child", ParentID: parent.ID})
	if err != nil {
		t.Fatalf("Create child: %v", err)
	}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	if err := cache.SetLocalString(child.ID, "synced_at", "value"); err != nil {
		t.Fatalf("SetLocalString: %v", err)
	}
	if err := backing.Delete(child.ID); err != nil {
		t.Fatalf("backing Delete: %v", err)
	}

	if _, err := cache.List(ListQuery{ParentID: parent.ID}); err != nil {
		t.Fatalf("List parent: %v", err)
	}

	requireNoCachedLocalMeta(t, cache, child.ID)
}

func TestCachingStoreTxInvalidatesTouchedCacheEntriesAfterCommit(t *testing.T) {
	backing := &txObservingBackingStore{Store: NewMemStore()}
	blocker, err := backing.Create(Bead{Title: "blocker"})
	if err != nil {
		t.Fatalf("Create blocker: %v", err)
	}
	first, err := backing.Create(Bead{Title: "first", Metadata: map[string]string{"phase": "old"}})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, err := backing.Create(Bead{Title: "second"})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if err := backing.DepAdd(first.ID, blocker.ID, "blocks"); err != nil {
		t.Fatalf("DepAdd: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	cache.mu.Lock()
	cache.dirty[first.ID] = struct{}{}
	cache.deletedSeq[first.ID] = 1
	cache.mu.Unlock()

	newTitle := "second in tx"
	if err := cache.Tx("cache tx", func(tx Tx) error {
		if err := tx.SetMetadataBatch(first.ID, map[string]string{"phase": "tx"}); err != nil {
			return err
		}
		return tx.Update(second.ID, UpdateOpts{Title: &newTitle})
	}); err != nil {
		t.Fatalf("Tx: %v", err)
	}

	if backing.txCalls != 1 {
		t.Fatalf("backing Tx calls = %d, want 1", backing.txCalls)
	}
	if backing.commitMsg != "cache tx" {
		t.Fatalf("commit message = %q, want cache tx", backing.commitMsg)
	}

	cache.mu.RLock()
	_, firstCached := cache.beads[first.ID]
	_, firstDepsCached := cache.deps[first.ID]
	_, firstDirty := cache.dirty[first.ID]
	_, firstDeleted := cache.deletedSeq[first.ID]
	_, firstMutated := cache.beadSeq[first.ID]
	firstLocalAt := cache.localBeadAt[first.ID]
	_, secondCached := cache.beads[second.ID]
	_, secondDepsCached := cache.deps[second.ID]
	_, blockerStillCached := cache.beads[blocker.ID]
	cache.mu.RUnlock()

	if firstCached || firstDepsCached || firstDirty || firstDeleted {
		t.Fatalf("first cache entries after Tx: bead=%v deps=%v dirty=%v deleted=%v, want all evicted", firstCached, firstDepsCached, firstDirty, firstDeleted)
	}
	if secondCached || secondDepsCached {
		t.Fatalf("second cache entries after Tx: bead=%v deps=%v, want evicted", secondCached, secondDepsCached)
	}
	if !firstMutated || firstLocalAt.IsZero() {
		t.Fatalf("local mutation markers: mutated=%v localAt=%v, want marked for stale event protection", firstMutated, firstLocalAt)
	}
	if !blockerStillCached {
		t.Fatal("untouched blocker bead was evicted")
	}

	gotFirst, err := cache.Get(first.ID)
	if err != nil {
		t.Fatalf("Get first: %v", err)
	}
	if gotFirst.Metadata["phase"] != "tx" {
		t.Fatalf("first metadata phase = %q, want tx", gotFirst.Metadata["phase"])
	}
	gotSecond, err := cache.Get(second.ID)
	if err != nil {
		t.Fatalf("Get second: %v", err)
	}
	if gotSecond.Title != newTitle {
		t.Fatalf("second title = %q, want %q", gotSecond.Title, newTitle)
	}
}

func TestCachingStoreTxLeavesCacheUnchangedOnBackingError(t *testing.T) {
	wantErr := errors.New("commit failed")
	backing := &txObservingBackingStore{Store: NewMemStore(), afterFn: wantErr}
	blocker, err := backing.Create(Bead{Title: "blocker"})
	if err != nil {
		t.Fatalf("Create blocker: %v", err)
	}
	bead, err := backing.Create(Bead{Title: "cached", Metadata: map[string]string{"phase": "old"}})
	if err != nil {
		t.Fatalf("Create bead: %v", err)
	}
	if err := backing.DepAdd(bead.ID, blocker.ID, "blocks"); err != nil {
		t.Fatalf("DepAdd: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}

	newTitle := "changed in failed tx"
	err = cache.Tx("failed tx", func(tx Tx) error {
		if err := tx.Update(bead.ID, UpdateOpts{Title: &newTitle}); err != nil {
			return err
		}
		return tx.SetMetadataBatch(bead.ID, map[string]string{"phase": "tx"})
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Tx error = %v, want %v", err, wantErr)
	}

	cache.mu.RLock()
	cached, cachedOK := cache.beads[bead.ID]
	deps, depsOK := cache.deps[bead.ID]
	cache.mu.RUnlock()
	if !cachedOK {
		t.Fatal("cached bead was evicted after failed Tx")
	}
	if cached.Title != "cached" || cached.Metadata["phase"] != "old" {
		t.Fatalf("cached bead after failed Tx = %+v, want original cached state", cached)
	}
	if !depsOK || len(deps) != 1 || deps[0].DependsOnID != blocker.ID {
		t.Fatalf("cached deps after failed Tx = %+v, want original dependency", deps)
	}
}

func TestCachingStoreTxZeroTouchLeavesCacheAndRunsWithoutMutex(t *testing.T) {
	backing := &txObservingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "cached"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cache := NewCachingStoreForTest(backing, nil)
	backing.onTxStart = func() {
		if !cache.mu.TryLock() {
			t.Fatal("backing Tx started while cache mutex was held")
		}
		cache.mu.Unlock()
	}
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	cache.mu.RLock()
	startSeq := cache.mutationSeq
	cache.mu.RUnlock()

	if err := cache.Tx("noop tx", func(Tx) error { return nil }); err != nil {
		t.Fatalf("Tx: %v", err)
	}

	if backing.txCalls != 1 {
		t.Fatalf("backing Tx calls = %d, want 1", backing.txCalls)
	}
	cache.mu.RLock()
	_, cached := cache.beads[bead.ID]
	_, depsCached := cache.deps[bead.ID]
	endSeq := cache.mutationSeq
	cache.mu.RUnlock()
	if !cached || !depsCached {
		t.Fatalf("zero-touch Tx evicted cache entries: bead=%v deps=%v", cached, depsCached)
	}
	if endSeq != startSeq {
		t.Fatalf("mutationSeq after zero-touch Tx = %d, want %d", endSeq, startSeq)
	}
}

// TestCachingStoreSetMetadataSkipsBackingWhenCachedValueMatches verifies that
// SetMetadata short-circuits before the backing call when the cached bead
// already has metadata[key]==value. Without this guard, no-op writes still
// fire bd's on_update hook and emit a bead.updated event.
func TestCachingStoreSetMetadataSkipsBackingWhenCachedValueMatches(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := backing.SetMetadata(bead.ID, "foo", "bar"); err != nil {
		t.Fatalf("seed SetMetadata: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.setMetadataCalls = 0

	if err := cache.SetMetadata(bead.ID, "foo", "bar"); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}
	if backing.setMetadataCalls != 0 {
		t.Errorf("backing.SetMetadata called %d times; want 0 (no-op write must short-circuit)",
			backing.setMetadataCalls)
	}
}

// TestCachingStoreSetMetadataFallsThroughOnValueMismatch verifies that a
// real value change still propagates to the backing store.
func TestCachingStoreSetMetadataFallsThroughOnValueMismatch(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := backing.SetMetadata(bead.ID, "foo", "old"); err != nil {
		t.Fatalf("seed SetMetadata: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.setMetadataCalls = 0

	if err := cache.SetMetadata(bead.ID, "foo", "new"); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}
	if backing.setMetadataCalls != 1 {
		t.Errorf("backing.SetMetadata called %d times; want 1 (real change must propagate)",
			backing.setMetadataCalls)
	}
}

// TestCachingStoreSetMetadataFallsThroughOnCacheMiss verifies that
// SetMetadata calls the backing store when the cache has no entry for the
// bead — without a primed copy we cannot prove the write is a no-op.
func TestCachingStoreSetMetadataFallsThroughOnCacheMiss(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}

	bead, err := backing.Create(Bead{Title: "post-prime"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	backing.setMetadataCalls = 0

	if err := cache.SetMetadata(bead.ID, "foo", "bar"); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}
	if backing.setMetadataCalls != 1 {
		t.Errorf("backing.SetMetadata called %d times; want 1 (cache miss must fall through)",
			backing.setMetadataCalls)
	}
}

// TestCachingStoreSetMetadataBatchSkipsBackingWhenAllCachedValuesMatch
// verifies that SetMetadataBatch short-circuits when every kv pair already
// matches the cached metadata.
func TestCachingStoreSetMetadataBatchSkipsBackingWhenAllCachedValuesMatch(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for k, v := range map[string]string{"foo": "1", "bar": "2", "baz": "3"} {
		if err := backing.SetMetadata(bead.ID, k, v); err != nil {
			t.Fatalf("seed SetMetadata(%s): %v", k, err)
		}
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.setMetadataBatchCalls = 0

	if err := cache.SetMetadataBatch(bead.ID, map[string]string{"foo": "1", "bar": "2"}); err != nil {
		t.Fatalf("SetMetadataBatch: %v", err)
	}
	if backing.setMetadataBatchCalls != 0 {
		t.Errorf("backing.SetMetadataBatch called %d times; want 0 (all-match must short-circuit)",
			backing.setMetadataBatchCalls)
	}
}

// TestCachingStoreSetMetadataBatchFallsThroughOnAnyMismatch verifies that
// even one mismatching kv forces the backing call — partial matches do not
// suffice to skip the write.
func TestCachingStoreSetMetadataBatchFallsThroughOnAnyMismatch(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for k, v := range map[string]string{"foo": "1", "bar": "2"} {
		if err := backing.SetMetadata(bead.ID, k, v); err != nil {
			t.Fatalf("seed SetMetadata(%s): %v", k, err)
		}
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.setMetadataBatchCalls = 0

	// foo matches the cached value, bar does not. The mismatch must force
	// the full batch to the backing store.
	if err := cache.SetMetadataBatch(bead.ID, map[string]string{"foo": "1", "bar": "DIFFERENT"}); err != nil {
		t.Fatalf("SetMetadataBatch: %v", err)
	}
	if backing.setMetadataBatchCalls != 1 {
		t.Errorf("backing.SetMetadataBatch called %d times; want 1 (mismatch must propagate)",
			backing.setMetadataBatchCalls)
	}
}

// TestCachingStoreSetMetadataBatchEmptyKVsIsNoop verifies that an empty kvs
// map returns nil immediately without calling the backing store. This is
// the early-return branch before metadataAlreadyMatchesCached.
func TestCachingStoreSetMetadataBatchEmptyKVsIsNoop(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.setMetadataBatchCalls = 0

	if err := cache.SetMetadataBatch(bead.ID, map[string]string{}); err != nil {
		t.Fatalf("SetMetadataBatch(empty): %v", err)
	}
	if backing.setMetadataBatchCalls != 0 {
		t.Errorf("backing.SetMetadataBatch called %d times; want 0 (empty kvs must short-circuit)",
			backing.setMetadataBatchCalls)
	}
}

// TestCachingStoreUpdateSkipsBackingWhenAllFieldsMatch verifies that Update
// short-circuits before the backing call when every non-nil opts field
// already matches the cached bead. Without this guard the reconciler's
// per-tick Update calls fire bd subprocesses + post-Get refreshes even when
// the payload is identical. See gastownhall/gascity#1978 Phase 1.
func TestCachingStoreUpdateSkipsBackingWhenAllFieldsMatch(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test", Assignee: "alice"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.updateCalls = 0

	assignee := "alice"
	if err := cache.Update(bead.ID, UpdateOpts{Assignee: &assignee}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if backing.updateCalls != 0 {
		t.Errorf("backing.Update called %d times; want 0 (no-op update must short-circuit)",
			backing.updateCalls)
	}
}

// TestCachingStoreUpdateFallsThroughOnValueMismatch verifies that a real
// field change still propagates to the backing store.
func TestCachingStoreUpdateFallsThroughOnValueMismatch(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test", Assignee: "alice"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.updateCalls = 0

	assignee := "bob"
	if err := cache.Update(bead.ID, UpdateOpts{Assignee: &assignee}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if backing.updateCalls != 1 {
		t.Errorf("backing.Update called %d times; want 1 (real change must propagate)",
			backing.updateCalls)
	}
}

// TestCachingStoreUpdateFallsThroughOnCacheMiss verifies that Update calls
// the backing store when the cache has no entry for the bead — without a
// primed copy we cannot prove the write is a no-op.
func TestCachingStoreUpdateFallsThroughOnCacheMiss(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}

	bead, err := backing.Create(Bead{Title: "post-prime", Assignee: "alice"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	backing.updateCalls = 0

	assignee := "alice"
	if err := cache.Update(bead.ID, UpdateOpts{Assignee: &assignee}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if backing.updateCalls != 1 {
		t.Errorf("backing.Update called %d times; want 1 (cache miss must fall through)",
			backing.updateCalls)
	}
}

// TestCachingStoreUpdateFallsThroughOnLabelMismatch verifies that a Labels
// opt requesting a label not yet on the bead still propagates to the backing
// store.
func TestCachingStoreUpdateFallsThroughOnLabelMismatch(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test", Labels: []string{"existing"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.updateCalls = 0

	if err := cache.Update(bead.ID, UpdateOpts{Labels: []string{"new-label"}}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if backing.updateCalls != 1 {
		t.Errorf("backing.Update called %d times; want 1 (new label must propagate)",
			backing.updateCalls)
	}
}

// TestCachingStoreCloseSkipsBackingWhenAlreadyClosed verifies that Close
// short-circuits before the backing call when the cached bead is already
// closed. The cache only holds active beads after Prime, so the close has
// to happen through CachingStore first to seed the closed status into the
// cache. See gastownhall/gascity#1978 Phase 1.
func TestCachingStoreCloseSkipsBackingWhenAlreadyClosed(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}

	// First close: open → closed, must propagate.
	if err := cache.Close(bead.ID); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if backing.closeCalls != 1 {
		t.Fatalf("backing.Close after first close = %d, want 1", backing.closeCalls)
	}
	backing.closeCalls = 0

	// Second close on the already-closed bead must short-circuit. The
	// reconciler / cleanup paths sometimes re-close the same bead on
	// retry; that should not generate fresh bd subprocess traffic.
	if err := cache.Close(bead.ID); err != nil {
		t.Fatalf("repeat Close: %v", err)
	}
	if backing.closeCalls != 0 {
		t.Errorf("backing.Close called %d times on repeat close; want 0 (already-closed must short-circuit)",
			backing.closeCalls)
	}
}

// TestCachingStoreCloseFallsThroughWhenOpen verifies that a real close still
// propagates to the backing store.
func TestCachingStoreCloseFallsThroughWhenOpen(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.closeCalls = 0

	if err := cache.Close(bead.ID); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if backing.closeCalls != 1 {
		t.Errorf("backing.Close called %d times; want 1 (open->closed must propagate)",
			backing.closeCalls)
	}
}

// TestCachingStoreCloseFallsThroughOnCacheMiss verifies that Close calls the
// backing store when the cache has no entry for the bead.
func TestCachingStoreCloseFallsThroughOnCacheMiss(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}

	bead, err := backing.Create(Bead{Title: "post-prime"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	backing.closeCalls = 0

	if err := cache.Close(bead.ID); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if backing.closeCalls != 1 {
		t.Errorf("backing.Close called %d times; want 1 (cache miss must fall through)",
			backing.closeCalls)
	}
}

// TestCachingStoreUpdateSkipsBackingPerFieldMatch is the per-field
// short-circuit coverage requested in gastownhall/gascity#2199. The original
// PR #2159 exercised Assignee + Labels-mismatch + cache-miss only; the
// remaining 6 field branches in updateMatchesCached were asserted by
// inspection. This table-driven test pins the short-circuit behavior for
// each field independently so a future refactor of any single check
// surfaces in CI.
func TestCachingStoreUpdateSkipsBackingPerFieldMatch(t *testing.T) {
	t.Parallel()

	type fieldCase struct {
		name string
		seed Bead
		opts UpdateOpts
	}
	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }

	cases := []fieldCase{
		{
			name: "Title",
			seed: Bead{Title: "pinned"},
			opts: UpdateOpts{Title: strPtr("pinned")},
		},
		{
			name: "Status",
			seed: Bead{Title: "x", Status: "open"},
			opts: UpdateOpts{Status: strPtr("open")},
		},
		{
			name: "Type",
			seed: Bead{Title: "x", Type: "task"},
			opts: UpdateOpts{Type: strPtr("task")},
		},
		{
			name: "Priority",
			seed: Bead{Title: "x", Priority: intPtr(2)},
			opts: UpdateOpts{Priority: intPtr(2)},
		},
		{
			name: "Description",
			seed: Bead{Title: "x", Description: "body"},
			opts: UpdateOpts{Description: strPtr("body")},
		},
		{
			name: "ParentID",
			seed: Bead{Title: "x", ParentID: "gc-parent"},
			opts: UpdateOpts{ParentID: strPtr("gc-parent")},
		},
		{
			name: "Metadata",
			seed: Bead{Title: "x", Metadata: map[string]string{"k": "v"}},
			opts: UpdateOpts{Metadata: map[string]string{"k": "v"}},
		},
		{
			name: "Labels-present",
			seed: Bead{Title: "x", Labels: []string{"a", "b"}},
			opts: UpdateOpts{Labels: []string{"a"}},
		},
		{
			name: "RemoveLabels-absent",
			seed: Bead{Title: "x", Labels: []string{"a"}},
			opts: UpdateOpts{RemoveLabels: []string{"z"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backing := &countingBackingStore{Store: NewMemStore()}
			bead, err := backing.Create(tc.seed)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			cache := NewCachingStoreForTest(backing, nil)
			if err := cache.Prime(context.Background()); err != nil {
				t.Fatalf("Prime: %v", err)
			}
			backing.updateCalls = 0

			if err := cache.Update(bead.ID, tc.opts); err != nil {
				t.Fatalf("Update: %v", err)
			}
			if backing.updateCalls != 0 {
				t.Errorf("backing.Update called %d times; want 0 (%s value-match must short-circuit)",
					backing.updateCalls, tc.name)
			}
		})
	}
}

// TestCachingStoreUpdateFallsThroughPerFieldMismatch is the mismatch-side
// companion to TestCachingStoreUpdateSkipsBackingPerFieldMatch. Each
// subtest asserts that a real change in the named field forces the
// backing call — guarding the matcher against accidentally returning true
// when a single field actually differs.
func TestCachingStoreUpdateFallsThroughPerFieldMismatch(t *testing.T) {
	t.Parallel()

	type fieldCase struct {
		name string
		seed Bead
		opts UpdateOpts
	}
	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }

	cases := []fieldCase{
		{
			name: "Title",
			seed: Bead{Title: "before"},
			opts: UpdateOpts{Title: strPtr("after")},
		},
		{
			name: "Status",
			seed: Bead{Title: "x", Status: "open"},
			opts: UpdateOpts{Status: strPtr("closed")},
		},
		{
			name: "Type",
			seed: Bead{Title: "x", Type: "task"},
			opts: UpdateOpts{Type: strPtr("epic")},
		},
		{
			name: "Priority",
			seed: Bead{Title: "x", Priority: intPtr(2)},
			opts: UpdateOpts{Priority: intPtr(3)},
		},
		{
			name: "Priority-nil-cached",
			seed: Bead{Title: "x"},
			opts: UpdateOpts{Priority: intPtr(2)},
		},
		{
			name: "Description",
			seed: Bead{Title: "x", Description: "before"},
			opts: UpdateOpts{Description: strPtr("after")},
		},
		{
			name: "ParentID",
			seed: Bead{Title: "x", ParentID: "gc-a"},
			opts: UpdateOpts{ParentID: strPtr("gc-b")},
		},
		{
			name: "Metadata-value",
			seed: Bead{Title: "x", Metadata: map[string]string{"k": "old"}},
			opts: UpdateOpts{Metadata: map[string]string{"k": "new"}},
		},
		{
			name: "Metadata-missing-key",
			seed: Bead{Title: "x"},
			opts: UpdateOpts{Metadata: map[string]string{"k": "v"}},
		},
		{
			name: "RemoveLabels-present",
			seed: Bead{Title: "x", Labels: []string{"a", "b"}},
			opts: UpdateOpts{RemoveLabels: []string{"a"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backing := &countingBackingStore{Store: NewMemStore()}
			bead, err := backing.Create(tc.seed)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			cache := NewCachingStoreForTest(backing, nil)
			if err := cache.Prime(context.Background()); err != nil {
				t.Fatalf("Prime: %v", err)
			}
			backing.updateCalls = 0

			if err := cache.Update(bead.ID, tc.opts); err != nil {
				t.Fatalf("Update: %v", err)
			}
			if backing.updateCalls != 1 {
				t.Errorf("backing.Update called %d times; want 1 (%s real change must propagate)",
					backing.updateCalls, tc.name)
			}
		})
	}
}
