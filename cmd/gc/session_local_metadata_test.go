package main

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
)

type localLifecycleCall struct {
	beadID string
	key    string
	value  string
}

type localLifecycleStore struct {
	*beads.MemStore

	local            map[string]map[string]string
	setLocalCalls    []localLifecycleCall
	getLocalCalls    []localLifecycleCall
	setLocalErr      error
	getLocalErr      error
	txActive         bool
	setLocalInsideTx bool
}

func newLocalLifecycleStore() *localLifecycleStore {
	return &localLifecycleStore{
		MemStore: beads.NewMemStore(),
		local:    make(map[string]map[string]string),
	}
}

func (s *localLifecycleStore) SetLocalString(beadID, key, value string) error {
	s.setLocalCalls = append(s.setLocalCalls, localLifecycleCall{beadID: beadID, key: key, value: value})
	if s.txActive {
		s.setLocalInsideTx = true
	}
	if s.setLocalErr != nil {
		return s.setLocalErr
	}
	if s.local[beadID] == nil {
		s.local[beadID] = make(map[string]string)
	}
	s.local[beadID][key] = value
	return nil
}

func (s *localLifecycleStore) GetLocalString(beadID, key string) (string, bool, error) {
	s.getLocalCalls = append(s.getLocalCalls, localLifecycleCall{beadID: beadID, key: key})
	if s.getLocalErr != nil {
		return "", false, s.getLocalErr
	}
	values := s.local[beadID]
	if values == nil {
		return "", false, nil
	}
	value, ok := values[key]
	return value, ok, nil
}

func (s *localLifecycleStore) Tx(commitMsg string, fn func(beads.Tx) error) error {
	s.txActive = true
	defer func() {
		s.txActive = false
	}()
	return s.MemStore.Tx(commitMsg, fn)
}

func TestSetLocalOrDurableWritesLifecycleKeysToLocalFirst(t *testing.T) {
	store := newLocalLifecycleStore()
	bead, err := store.Create(beads.Bead{Title: "session", Type: sessionBeadType, Labels: []string{sessionBeadLabel}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var stderr bytes.Buffer
	if err := setLocalOrDurable(store, bead.ID, "synced_at", "2026-05-17T22:30:00Z", &stderr); err != nil {
		t.Fatalf("setLocalOrDurable: %v", err)
	}

	if got := store.local[bead.ID]["synced_at"]; got != "2026-05-17T22:30:00Z" {
		t.Fatalf("local synced_at = %q, want timestamp", got)
	}
	stored, err := store.Get(bead.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got := stored.Metadata["synced_at"]; got != "" {
		t.Fatalf("durable synced_at = %q, want empty", got)
	}
}

func TestSetLocalOrDurableFallsBackToDurableWhenUnsupported(t *testing.T) {
	store := beads.NewMemStore()
	bead, err := store.Create(beads.Bead{Title: "session", Type: sessionBeadType, Labels: []string{sessionBeadLabel}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var stderr bytes.Buffer
	if err := setLocalOrDurable(store, bead.ID, "last_woke_at", "2026-05-17T22:31:00Z", &stderr); err != nil {
		t.Fatalf("setLocalOrDurable: %v", err)
	}

	stored, err := store.Get(bead.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got := stored.Metadata["last_woke_at"]; got != "2026-05-17T22:31:00Z" {
		t.Fatalf("durable last_woke_at = %q, want timestamp", got)
	}
}

func TestSetLocalOrDurableDoesNotFallbackOnLocalWriteErrors(t *testing.T) {
	wantErr := errors.New("local metadata failed")
	store := newLocalLifecycleStore()
	store.setLocalErr = wantErr
	bead, err := store.Create(beads.Bead{Title: "session", Type: sessionBeadType, Labels: []string{sessionBeadLabel}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var stderr bytes.Buffer
	if err := setLocalOrDurable(store, bead.ID, "pending_create_claim", "true", &stderr); !errors.Is(err, wantErr) {
		t.Fatalf("setLocalOrDurable error = %v, want %v", err, wantErr)
	}
	stored, err := store.Get(bead.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got := stored.Metadata["pending_create_claim"]; got != "" {
		t.Fatalf("durable pending_create_claim = %q, want empty", got)
	}
}

func TestGetLocalOrDurablePrefersLocalAndFallsBackOnCacheMiss(t *testing.T) {
	store := newLocalLifecycleStore()
	bead, err := store.Create(beads.Bead{
		Title:  "session",
		Type:   sessionBeadType,
		Labels: []string{sessionBeadLabel},
		Metadata: map[string]string{
			"pending_create_claim": "durable",
			"last_woke_at":         "durable-wake",
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	store.local[bead.ID] = map[string]string{"pending_create_claim": "true"}

	value, ok, err := getLocalOrDurable(store, bead, "pending_create_claim")
	if err != nil {
		t.Fatalf("getLocalOrDurable local: %v", err)
	}
	if !ok || value != "true" {
		t.Fatalf("getLocalOrDurable local = %q, %v; want true, true", value, ok)
	}
	value, ok, err = getLocalOrDurable(store, bead, "last_woke_at")
	if err != nil {
		t.Fatalf("getLocalOrDurable durable: %v", err)
	}
	if !ok || value != "durable-wake" {
		t.Fatalf("getLocalOrDurable durable = %q, %v; want durable-wake, true", value, ok)
	}
}

func TestLoadSessionBeadSnapshotHydratesLocalLifecycleMetadata(t *testing.T) {
	store := newLocalLifecycleStore()
	bead, err := store.Create(beads.Bead{
		Title:  "session",
		Type:   sessionBeadType,
		Labels: []string{sessionBeadLabel},
		Metadata: map[string]string{
			"session_name":         "worker",
			"pending_create_claim": "",
			"last_woke_at":         "durable-wake",
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	store.local[bead.ID] = map[string]string{
		"pending_create_claim": "true",
		"last_woke_at":         "local-wake",
	}

	snapshot, err := loadSessionBeadSnapshot(store)
	if err != nil {
		t.Fatalf("loadSessionBeadSnapshot: %v", err)
	}
	open := snapshot.Open()
	if len(open) != 1 {
		t.Fatalf("open session count = %d, want 1", len(open))
	}
	if got := open[0].Metadata["pending_create_claim"]; got != "true" {
		t.Fatalf("pending_create_claim = %q, want local true", got)
	}
	if got := open[0].Metadata["last_woke_at"]; got != "local-wake" {
		t.Fatalf("last_woke_at = %q, want local-wake", got)
	}
}

func TestReopenClosedConfiguredNamedSessionBeadWritesLocalLifecycleMetadataAfterTx(t *testing.T) {
	cityPath := t.TempDir()
	store := newLocalLifecycleStore()
	now := time.Date(2026, 5, 17, 15, 31, 0, 0, time.UTC)
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Agents: []config.Agent{
			{Name: "refinery", StartCommand: "true", MaxActiveSessions: intPtr(2)},
		},
		NamedSessions: []config.NamedSession{
			{Template: "refinery", Mode: "on_demand"},
		},
	}
	sessionName := config.NamedSessionRuntimeName(cfg.Workspace.Name, cfg.Workspace, "refinery")
	closed, err := store.Create(beads.Bead{
		Title:  "refinery",
		Type:   sessionBeadType,
		Labels: []string{sessionBeadLabel},
		Metadata: map[string]string{
			"session_name":               sessionName,
			"alias":                      "refinery",
			"template":                   "refinery",
			"state":                      "suspended",
			"close_reason":               "suspended",
			namedSessionMetadataKey:      "true",
			namedSessionIdentityMetadata: "refinery",
			namedSessionModeMetadata:     "on_demand",
		},
	})
	if err != nil {
		t.Fatalf("create closed canonical bead: %v", err)
	}
	if err := store.Close(closed.ID); err != nil {
		t.Fatalf("close canonical bead: %v", err)
	}

	var stderr bytes.Buffer
	reopened, ok := reopenClosedConfiguredNamedSessionBead(
		cityPath, store, cfg, "test-city", "refinery", sessionName, "creating", now, nil, &stderr,
	)
	if !ok {
		t.Fatalf("reopenClosedConfiguredNamedSessionBead failed: %s", stderr.String())
	}
	if store.setLocalInsideTx {
		t.Fatal("SetLocalString was called inside Store.Tx")
	}
	if got := store.local[closed.ID]["pending_create_claim"]; got != "true" {
		t.Fatalf("local pending_create_claim = %q, want true", got)
	}
	if got := store.local[closed.ID]["synced_at"]; got != now.Format("2006-01-02T15:04:05Z07:00") {
		t.Fatalf("local synced_at = %q, want %q", got, now.Format("2006-01-02T15:04:05Z07:00"))
	}
	if reopened.Metadata["pending_create_claim"] != "true" {
		t.Fatalf("reopened pending_create_claim = %q, want true", reopened.Metadata["pending_create_claim"])
	}
	stored, err := store.Get(closed.ID)
	if err != nil {
		t.Fatalf("Get(%s): %v", closed.ID, err)
	}
	if got := stored.Metadata["pending_create_claim"]; got != "" {
		t.Fatalf("durable pending_create_claim = %q, want empty", got)
	}
	if got := stored.Metadata["synced_at"]; got != "" {
		t.Fatalf("durable synced_at = %q, want empty", got)
	}
}
