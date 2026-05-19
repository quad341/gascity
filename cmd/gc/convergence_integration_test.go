package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/convergence"
	"github.com/gastownhall/gascity/internal/events"
	"github.com/gastownhall/gascity/internal/runtime"
)

// setupConvergenceRuntime creates a CityRuntime with a MemStore and
// convergence handler initialized, suitable for integration tests.
// No socket is started — tests interact via handleConvergenceRequest
// or the convergenceReqCh channel.
func setupConvergenceRuntime(t *testing.T) (*CityRuntime, *beads.MemStore) {
	t.Helper()

	store := beads.NewMemStore()
	cfg := &config.City{
		Workspace:     config.Workspace{Name: "test"},
		FormulaLayers: config.FormulaLayers{City: []string{sharedTestFormulaDir}},
	}
	sp := runtime.NewFake()
	convergenceReqCh := make(chan convergenceRequest, 16)

	cr := &CityRuntime{
		cityPath:            t.TempDir(),
		cityName:            "test",
		cfg:                 cfg,
		sp:                  sp,
		buildFn:             func(_ *config.City, _ runtime.Provider, _ beads.Store) map[string]TemplateParams { return nil },
		rec:                 events.Discard,
		convergenceReqCh:    convergenceReqCh,
		standaloneCityStore: store,
		logPrefix:           "gc test",
		stdout:              &bytes.Buffer{},
		stderr:              &bytes.Buffer{},
	}

	// Initialize convergence handlers (mimics initConvergenceHandlers).
	cr.initConvergenceHandlers()

	return cr, store
}

// sendAndReceive sends a convergence request via handleConvergenceRequest
// and returns the reply.
func sendAndReceive(t *testing.T, cr *CityRuntime, req convergenceRequest) convergenceReply {
	t.Helper()
	return cr.handleConvergenceRequest(context.Background(), req)
}

// --- Channel-level tests ---

func TestConvergence_CreateReply(t *testing.T) {
	cr, _ := setupConvergenceRuntime(t)

	reply := sendAndReceive(t, cr, convergenceRequest{
		Command: "create",
		Params: map[string]string{
			"formula":        "test-formula",
			"target":         "test-agent",
			"max_iterations": "3",
		},
	})
	if reply.Error != "" {
		t.Fatalf("unexpected error: %s", reply.Error)
	}

	var result convergence.CreateResult
	if err := json.Unmarshal(reply.Result, &result); err != nil {
		t.Fatalf("unmarshaling result: %v", err)
	}
	if result.BeadID == "" {
		t.Error("expected non-empty bead ID")
	}
	if result.FirstWispID == "" {
		t.Error("expected non-empty first wisp ID")
	}
}

func TestConvergence_StopCommand(t *testing.T) {
	cr, _ := setupConvergenceRuntime(t)

	// Create a loop first.
	createReply := sendAndReceive(t, cr, convergenceRequest{
		Command: "create",
		Params: map[string]string{
			"formula":        "test-formula",
			"target":         "test-agent",
			"max_iterations": "5",
		},
	})
	if createReply.Error != "" {
		t.Fatalf("create error: %s", createReply.Error)
	}
	var created convergence.CreateResult
	if err := json.Unmarshal(createReply.Result, &created); err != nil {
		t.Fatalf("unmarshaling create result: %v", err)
	}

	// Stop the loop.
	stopReply := sendAndReceive(t, cr, convergenceRequest{
		Command: "stop",
		BeadID:  created.BeadID,
		User:    "test-operator",
	})
	if stopReply.Error != "" {
		t.Fatalf("stop error: %s", stopReply.Error)
	}

	// Verify state is terminated.
	meta, err := cr.cityConvergenceHandler().Store.GetMetadata(created.BeadID)
	if err != nil {
		t.Fatalf("GetMetadata: %v", err)
	}
	if meta[convergence.FieldState] != convergence.StateTerminated {
		t.Errorf("state = %q, want %q", meta[convergence.FieldState], convergence.StateTerminated)
	}
}

func TestConvergence_UnknownCommand(t *testing.T) {
	cr, _ := setupConvergenceRuntime(t)

	reply := sendAndReceive(t, cr, convergenceRequest{
		Command: "bogus",
	})
	if reply.Error == "" {
		t.Fatal("expected error for unknown command")
	}
}

func TestConvergence_PanicRecovery(t *testing.T) {
	cr, _ := setupConvergenceRuntime(t)

	// Temporarily remove convergence handlers to exercise the guarded error
	// path used by socket command processing.
	savedHandlers := cr.convHandlers
	cr.convHandlers = nil

	reply := cr.safeHandleConvergenceRequest(context.Background(), convergenceRequest{
		Command: "approve",
		BeadID:  "nonexistent",
	})
	// safeHandleConvergenceRequest should return error, not panic.
	if reply.Error == "" {
		t.Error("expected error reply from nil handler")
	}

	cr.convHandlers = savedHandlers
}

func TestConvergence_TickProcessesClosedWisp(t *testing.T) {
	cr, store := setupConvergenceRuntime(t)

	// Create a convergence loop.
	createReply := sendAndReceive(t, cr, convergenceRequest{
		Command: "create",
		Params: map[string]string{
			"formula":        "test-formula",
			"target":         "test-agent",
			"max_iterations": "5",
		},
	})
	if createReply.Error != "" {
		t.Fatalf("create error: %s", createReply.Error)
	}
	var created convergence.CreateResult
	if err := json.Unmarshal(createReply.Result, &created); err != nil {
		t.Fatalf("unmarshaling: %v", err)
	}

	// Populate the active index so convergenceTick works.
	adapter := cr.cityConvergenceHandler().Store.(*convergenceStoreAdapter)
	if err := adapter.populateIndex(); err != nil {
		t.Fatalf("populateIndex: %v", err)
	}

	// Close the active wisp to simulate it finishing.
	if err := store.Close(created.FirstWispID); err != nil {
		t.Fatalf("closing wisp: %v", err)
	}

	// Run convergenceTick — it should detect the closed wisp and process it.
	cr.convergenceTick(context.Background())

	// After processing, active_wisp should have changed (iterated to next wisp
	// or terminated, depending on gate mode — manual mode transitions to waiting_manual).
	meta, _ := cr.cityConvergenceHandler().Store.GetMetadata(created.BeadID)
	state := meta[convergence.FieldState]
	// With manual gate mode, closing a wisp transitions to waiting_manual.
	if state != convergence.StateWaitingManual {
		t.Errorf("state after tick = %q, want %q", state, convergence.StateWaitingManual)
	}
}

func TestConvergence_StartupReconcile(t *testing.T) {
	cr, store := setupConvergenceRuntime(t)

	// Create a convergence bead that looks like it was interrupted mid-creation.
	b, err := store.Create(beads.Bead{
		Title:  "interrupted",
		Type:   "convergence",
		Status: "in_progress",
	})
	if err != nil {
		t.Fatalf("creating bead: %v", err)
	}
	if err := store.SetMetadata(b.ID, convergence.FieldState, convergence.StateCreating); err != nil {
		t.Fatalf("setting state: %v", err)
	}

	// Run startup reconcile.
	cr.convergenceStartupReconcile(context.Background())

	// The bead should now be terminated and closed.
	updated, err := store.Get(b.ID)
	if err != nil {
		t.Fatalf("getting bead: %v", err)
	}
	if updated.Status != "closed" {
		t.Errorf("bead status = %q, want %q", updated.Status, "closed")
	}
	if updated.Metadata[convergence.FieldState] != convergence.StateTerminated {
		t.Errorf("state = %q, want %q", updated.Metadata[convergence.FieldState], convergence.StateTerminated)
	}

	// The active index should be populated after startup reconcile.
	adapter := cr.cityConvergenceHandler().Store.(*convergenceStoreAdapter)
	if adapter.activeIndex == nil {
		t.Error("active index should be populated after startup reconcile")
	}
}

func TestConvergence_EnqueueTimeout(t *testing.T) {
	cr, _ := setupConvergenceRuntime(t)

	// Fill the channel to capacity.
	for i := 0; i < cap(cr.convergenceReqCh); i++ {
		cr.convergenceReqCh <- convergenceRequest{
			Command: "create",
			replyCh: make(chan convergenceReply, 1),
		}
	}

	// Try to send one more — should not block (we use a select with timeout).
	done := make(chan bool, 1)
	go func() {
		select {
		case cr.convergenceReqCh <- convergenceRequest{
			Command: "create",
			replyCh: make(chan convergenceReply, 1),
		}:
			done <- false // should not succeed immediately
		case <-time.After(50 * time.Millisecond):
			done <- true // timeout is expected
		}
	}()

	select {
	case timedOut := <-done:
		if !timedOut {
			t.Error("expected channel send to block when full")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("test timed out")
	}

	// Drain the channel.
	for len(cr.convergenceReqCh) > 0 {
		<-cr.convergenceReqCh
	}
}

func TestInitConvergenceHandlersBuildsCityAndRigHandlers(t *testing.T) {
	cityStore := beads.NewMemStore()
	rigStore := beads.NewMemStore()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test"},
		Rigs: []config.Rig{
			{Name: "rig-a"},
		},
		FormulaLayers: config.FormulaLayers{
			City: []string{"/city-formulas"},
			Rigs: map[string][]string{
				"rig-a": {"/rig-a-formulas"},
			},
		},
	}
	cr := &CityRuntime{
		cfg:                 cfg,
		rec:                 events.Discard,
		convergenceReqCh:    make(chan convergenceRequest, 1),
		standaloneCityStore: cityStore,
		stderr:              &bytes.Buffer{},
	}
	cs := &controllerState{
		cfg:           cfg,
		beadStores:    map[string]beads.Store{"rig-a": rigStore},
		cityBeadStore: cityStore,
	}
	cr.setControllerState(cs)

	cr.initConvergenceHandlers()

	if len(cr.convHandlers) != 2 {
		t.Fatalf("convHandlers len = %d, want 2", len(cr.convHandlers))
	}
	if cr.convHandlers[""] == nil {
		t.Fatal("city convergence handler not initialized at empty key")
	}
	if cr.convHandlers["rig-a"] == nil {
		t.Fatal("rig convergence handler not initialized at rig name key")
	}
	if got, want := cr.convStoreAdapters[""].formulaSearchPaths, []string{"/city-formulas"}; !equalStrings(got, want) {
		t.Fatalf("city formulaSearchPaths = %v, want %v", got, want)
	}
	if got, want := cr.convStoreAdapters["rig-a"].formulaSearchPaths, []string{"/rig-a-formulas"}; !equalStrings(got, want) {
		t.Fatalf("rig formulaSearchPaths = %v, want %v", got, want)
	}
}

func TestInitConvergenceHandlersSkipsFailingRigStore(t *testing.T) {
	cityStore := beads.NewMemStore()
	healthyRigStore := beads.NewMemStore()
	failingRigStore := pingFailStore{
		Store: beads.NewMemStore(),
		err:   errors.New("rig db unavailable"),
	}
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test"},
		Rigs: []config.Rig{
			{Name: "bad-rig"},
			{Name: "good-rig"},
		},
	}
	var stderr bytes.Buffer
	cr := &CityRuntime{
		cfg:                 cfg,
		rec:                 events.Discard,
		convergenceReqCh:    make(chan convergenceRequest, 1),
		standaloneCityStore: cityStore,
		logPrefix:           "gc test",
		stderr:              &stderr,
	}
	cs := &controllerState{
		cfg: cfg,
		beadStores: map[string]beads.Store{
			"bad-rig":  failingRigStore,
			"good-rig": healthyRigStore,
		},
		cityBeadStore: cityStore,
	}
	cr.setControllerState(cs)

	cr.initConvergenceHandlers()

	if cr.convHandlers[""] == nil {
		t.Fatal("city convergence handler should remain initialized")
	}
	if cr.convHandlers["good-rig"] == nil {
		t.Fatal("healthy rig convergence handler should be initialized")
	}
	if cr.convHandlers["bad-rig"] != nil {
		t.Fatal("failing rig convergence handler should be skipped")
	}
	if got := stderr.String(); !strings.Contains(got, `gc test: convergence: rig "bad-rig" unavailable`) || !strings.Contains(got, "rig db unavailable") {
		t.Fatalf("stderr = %q, want failing rig init log", got)
	}
}

func TestHandleConvergenceCreateRoutesToRigHandler(t *testing.T) {
	cr, cityStore, rigStore := setupConvergenceRuntimeWithRig(t)

	reply := sendAndReceive(t, cr, convergenceRequest{
		Command: "create",
		Params: map[string]string{
			"formula":        "test-formula",
			"target":         "rig-agent",
			"max_iterations": "3",
			"rig":            "rig-a",
		},
	})
	if reply.Error != "" {
		t.Fatalf("create error: %s", reply.Error)
	}
	var created convergence.CreateResult
	if err := json.Unmarshal(reply.Result, &created); err != nil {
		t.Fatalf("unmarshaling create result: %v", err)
	}
	if _, err := rigStore.Get(created.BeadID); err != nil {
		t.Fatalf("created bead missing from rig store: %v", err)
	}
	if _, err := cityStore.Get(created.BeadID); err == nil {
		t.Fatalf("created bead %q should not be in city store", created.BeadID)
	}
}

func TestHandleConvergenceCreateUnknownRig(t *testing.T) {
	cr, _, _ := setupConvergenceRuntimeWithRig(t)

	reply := sendAndReceive(t, cr, convergenceRequest{
		Command: "create",
		Params: map[string]string{
			"formula":        "test-formula",
			"target":         "rig-agent",
			"max_iterations": "3",
			"rig":            "missing-rig",
		},
	})
	want := `rig "missing-rig" is not available; check that it is configured and that its store is reachable`
	if reply.Error != want {
		t.Fatalf("reply error = %q, want %q", reply.Error, want)
	}
}

func TestConvergenceCreatedEventsCarryStoreKeyOnlyForRigStore(t *testing.T) {
	rec := events.NewFake()
	cr, _, _ := setupConvergenceRuntimeWithRigRecorder(t, rec)
	cityCreated := createConvergenceLoop(t, cr, "", "city-agent")
	cityEvent := findRecordedConvergenceCreatedEvent(t, rec, cityCreated.BeadID)
	var cityRaw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(cityEvent.Message), &cityRaw); err != nil {
		t.Fatalf("unmarshal city created event message: %v", err)
	}
	if _, ok := cityRaw["store_key"]; ok {
		t.Fatalf("city created event should omit store_key: %s", cityEvent.Message)
	}

	rigCreated := createConvergenceLoop(t, cr, "rig-a", "rig-agent")
	rigEvent := findRecordedConvergenceCreatedEvent(t, rec, rigCreated.BeadID)
	var rigPayload convergence.CreatedPayload
	if err := json.Unmarshal([]byte(rigEvent.Message), &rigPayload); err != nil {
		t.Fatalf("unmarshal rig created event message: %v", err)
	}
	if rigPayload.StoreKey != "rig-a" {
		t.Fatalf("rig created event store_key = %q, want %q", rigPayload.StoreKey, "rig-a")
	}
}

func TestConvergenceCommandFindsRigBeadByID(t *testing.T) {
	cr, _, rigStore := setupConvergenceRuntimeWithRig(t)
	created := createConvergenceLoop(t, cr, "rig-a", "rig-agent")

	reply := sendAndReceive(t, cr, convergenceRequest{
		Command: "stop",
		BeadID:  created.BeadID,
		User:    "test-operator",
	})
	if reply.Error != "" {
		t.Fatalf("stop error: %s", reply.Error)
	}

	updated, err := rigStore.Get(created.BeadID)
	if err != nil {
		t.Fatalf("getting rig bead: %v", err)
	}
	if updated.Status != "closed" {
		t.Fatalf("rig bead status = %q, want closed", updated.Status)
	}
	if got := updated.Metadata[convergence.FieldTerminalReason]; got != convergence.TerminalStopped {
		t.Fatalf("terminal_reason = %q, want %q", got, convergence.TerminalStopped)
	}
}

func TestConvergenceTickProcessesAllHandlers(t *testing.T) {
	cr, cityStore, rigStore := setupConvergenceRuntimeWithRig(t)
	cityCreated := createConvergenceLoop(t, cr, "", "city-agent")
	rigCreated := createConvergenceLoop(t, cr, "rig-a", "rig-agent")

	for key, adapter := range cr.convStoreAdapters {
		if err := adapter.populateIndex(); err != nil {
			t.Fatalf("populateIndex(%q): %v", key, err)
		}
	}
	if err := cityStore.Close(cityCreated.FirstWispID); err != nil {
		t.Fatalf("closing city wisp: %v", err)
	}
	if err := rigStore.Close(rigCreated.FirstWispID); err != nil {
		t.Fatalf("closing rig wisp: %v", err)
	}

	cr.convergenceTick(context.Background())

	cityMeta, err := cr.convStoreAdapters[""].GetMetadata(cityCreated.BeadID)
	if err != nil {
		t.Fatalf("city metadata: %v", err)
	}
	rigMeta, err := cr.convStoreAdapters["rig-a"].GetMetadata(rigCreated.BeadID)
	if err != nil {
		t.Fatalf("rig metadata: %v", err)
	}
	if cityMeta[convergence.FieldState] != convergence.StateWaitingManual {
		t.Fatalf("city state = %q, want %q", cityMeta[convergence.FieldState], convergence.StateWaitingManual)
	}
	if rigMeta[convergence.FieldState] != convergence.StateWaitingManual {
		t.Fatalf("rig state = %q, want %q", rigMeta[convergence.FieldState], convergence.StateWaitingManual)
	}
}

func TestRigConvergenceLoopCreateTickTerminates(t *testing.T) {
	cr, cityStore, rigStore := setupConvergenceRuntimeWithRig(t)
	gatePath := writePassingGate(t, cr.cityPath)

	reply := sendAndReceive(t, cr, convergenceRequest{
		Command: "create",
		Params: map[string]string{
			"formula":        "test-formula",
			"target":         "rig-agent",
			"max_iterations": "3",
			"rig":            "rig-a",
			"gate_mode":      convergence.GateModeCondition,
			"gate_condition": gatePath,
		},
	})
	if reply.Error != "" {
		t.Fatalf("create error: %s", reply.Error)
	}
	var created convergence.CreateResult
	if err := json.Unmarshal(reply.Result, &created); err != nil {
		t.Fatalf("unmarshaling create result: %v", err)
	}
	if _, err := cityStore.Get(created.BeadID); err == nil {
		t.Fatalf("created bead %q should not be in city store", created.BeadID)
	}
	if err := cr.convStoreAdapters["rig-a"].populateIndex(); err != nil {
		t.Fatalf("populateIndex: %v", err)
	}
	if err := rigStore.Close(created.FirstWispID); err != nil {
		t.Fatalf("closing rig wisp: %v", err)
	}

	cr.convergenceTick(context.Background())

	updated, err := rigStore.Get(created.BeadID)
	if err != nil {
		t.Fatalf("getting rig convergence bead: %v", err)
	}
	if updated.Status != "closed" {
		t.Fatalf("rig convergence status = %q, want closed", updated.Status)
	}
	if updated.Metadata[convergence.FieldState] != convergence.StateTerminated {
		t.Fatalf("rig convergence state = %q, want %q", updated.Metadata[convergence.FieldState], convergence.StateTerminated)
	}
	if updated.Metadata[convergence.FieldTerminalReason] != convergence.TerminalApproved {
		t.Fatalf("terminal_reason = %q, want %q", updated.Metadata[convergence.FieldTerminalReason], convergence.TerminalApproved)
	}
	if updated.Metadata[convergence.FieldGateOutcome] != convergence.GatePass {
		t.Fatalf("gate_outcome = %q, want %q", updated.Metadata[convergence.FieldGateOutcome], convergence.GatePass)
	}
}

func TestConvergenceStartupReconcileScansAllStores(t *testing.T) {
	cr, cityStore, rigStore := setupConvergenceRuntimeWithRig(t)
	cityInterrupted := createInterruptedConvergenceBead(t, cityStore, "city interrupted")
	rigInterrupted := createInterruptedConvergenceBead(t, rigStore, "rig interrupted")

	cr.convergenceStartupReconcile(context.Background())

	assertTerminatedClosed(t, cityStore, cityInterrupted)
	assertTerminatedClosed(t, rigStore, rigInterrupted)
	if cr.convStoreAdapters[""].activeIndex == nil {
		t.Fatal("city active index was not populated")
	}
	if cr.convStoreAdapters["rig-a"].activeIndex == nil {
		t.Fatal("rig active index was not populated")
	}
}

func setupConvergenceRuntimeWithRig(t *testing.T) (*CityRuntime, *beads.MemStore, *beads.MemStore) {
	t.Helper()
	return setupConvergenceRuntimeWithRigRecorder(t, events.Discard)
}

func setupConvergenceRuntimeWithRigRecorder(t *testing.T, rec events.Recorder) (*CityRuntime, *beads.MemStore, *beads.MemStore) {
	t.Helper()

	cityStore := beads.NewMemStore()
	rigStore := beads.NewMemStore()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test"},
		Rigs: []config.Rig{
			{Name: "rig-a"},
		},
		FormulaLayers: config.FormulaLayers{
			City: []string{sharedTestFormulaDir},
			Rigs: map[string][]string{
				"rig-a": {sharedTestFormulaDir},
			},
		},
	}
	cr := &CityRuntime{
		cityPath:            t.TempDir(),
		cityName:            "test",
		cfg:                 cfg,
		sp:                  runtime.NewFake(),
		buildFn:             func(_ *config.City, _ runtime.Provider, _ beads.Store) map[string]TemplateParams { return nil },
		rec:                 rec,
		convergenceReqCh:    make(chan convergenceRequest, 16),
		standaloneCityStore: cityStore,
		logPrefix:           "gc test",
		stdout:              &bytes.Buffer{},
		stderr:              &bytes.Buffer{},
	}
	cs := &controllerState{
		cfg: cfg,
		beadStores: map[string]beads.Store{
			"rig-a": rigStore,
		},
		cityBeadStore: cityStore,
	}
	cr.setControllerState(cs)
	cr.initConvergenceHandlers()
	return cr, cityStore, rigStore
}

func writePassingGate(t *testing.T, cityPath string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(cityPath, "gates"), 0o755); err != nil {
		t.Fatalf("creating gate dir: %v", err)
	}
	scriptPath := filepath.Join(cityPath, "gates", "pass.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("writing gate script: %v", err)
	}
	return filepath.Join("gates", "pass.sh")
}

func findRecordedConvergenceCreatedEvent(t *testing.T, rec *events.Fake, beadID string) events.Event {
	t.Helper()
	recordedEvents, err := rec.List(events.Filter{Type: convergence.EventCreated})
	if err != nil {
		t.Fatalf("list recorded events: %v", err)
	}
	for i := len(recordedEvents) - 1; i >= 0; i-- {
		event := recordedEvents[i]
		if event.Type == convergence.EventCreated && event.Subject == beadID {
			return event
		}
	}
	t.Fatalf("recorded %s event for %s not found in %#v", convergence.EventCreated, beadID, recordedEvents)
	return events.Event{}
}

func createConvergenceLoop(t *testing.T, cr *CityRuntime, rigName, target string) convergence.CreateResult {
	t.Helper()
	params := map[string]string{
		"formula":        "test-formula",
		"target":         target,
		"max_iterations": "3",
	}
	if rigName != "" {
		params["rig"] = rigName
	}
	reply := sendAndReceive(t, cr, convergenceRequest{
		Command: "create",
		Params:  params,
	})
	if reply.Error != "" {
		t.Fatalf("create %q error: %s", rigName, reply.Error)
	}
	var created convergence.CreateResult
	if err := json.Unmarshal(reply.Result, &created); err != nil {
		t.Fatalf("unmarshaling create result: %v", err)
	}
	return created
}

func createInterruptedConvergenceBead(t *testing.T, store *beads.MemStore, title string) string {
	t.Helper()
	b, err := store.Create(beads.Bead{
		Title:  title,
		Type:   "convergence",
		Status: "in_progress",
	})
	if err != nil {
		t.Fatalf("creating interrupted bead: %v", err)
	}
	if err := store.SetMetadata(b.ID, convergence.FieldState, convergence.StateCreating); err != nil {
		t.Fatalf("setting state: %v", err)
	}
	return b.ID
}

func assertTerminatedClosed(t *testing.T, store *beads.MemStore, beadID string) {
	t.Helper()
	updated, err := store.Get(beadID)
	if err != nil {
		t.Fatalf("getting bead %q: %v", beadID, err)
	}
	if updated.Status != "closed" {
		t.Fatalf("%s status = %q, want closed", beadID, updated.Status)
	}
	if updated.Metadata[convergence.FieldState] != convergence.StateTerminated {
		t.Fatalf("%s state = %q, want %q", beadID, updated.Metadata[convergence.FieldState], convergence.StateTerminated)
	}
}

type pingFailStore struct {
	beads.Store
	err error
}

func (s pingFailStore) Ping() error {
	return fmt.Errorf("ping failed: %w", s.err)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- Active index tests ---

func TestConvergenceIndex_PopulateAndQuery(t *testing.T) {
	store := beads.NewMemStore()
	adapter := newConvergenceStoreAdapter(store, nil)

	// Create some convergence beads in various states.
	active, _ := store.Create(beads.Bead{Title: "active", Type: "convergence", Status: "in_progress"})
	_ = store.SetMetadata(active.ID, convergence.FieldState, convergence.StateActive)
	_ = store.SetMetadata(active.ID, convergence.FieldTarget, "agent-1")

	waiting, _ := store.Create(beads.Bead{Title: "waiting", Type: "convergence", Status: "in_progress"})
	_ = store.SetMetadata(waiting.ID, convergence.FieldState, convergence.StateWaitingManual)
	_ = store.SetMetadata(waiting.ID, convergence.FieldTarget, "agent-2")

	terminated, _ := store.Create(beads.Bead{Title: "terminated", Type: "convergence", Status: "closed"})
	_ = store.SetMetadata(terminated.ID, convergence.FieldState, convergence.StateTerminated)

	if err := adapter.populateIndex(); err != nil {
		t.Fatalf("populateIndex: %v", err)
	}

	ids := adapter.activeBeadIDs()
	if len(ids) != 2 {
		t.Errorf("activeBeadIDs count = %d, want 2", len(ids))
	}

	// CountActiveConvergenceLoops should use the index.
	count1, _ := adapter.CountActiveConvergenceLoops("agent-1")
	if count1 != 1 {
		t.Errorf("count for agent-1 = %d, want 1", count1)
	}
	count2, _ := adapter.CountActiveConvergenceLoops("agent-2")
	if count2 != 1 {
		t.Errorf("count for agent-2 = %d, want 1", count2)
	}
	count3, _ := adapter.CountActiveConvergenceLoops("no-such-agent")
	if count3 != 0 {
		t.Errorf("count for no-such-agent = %d, want 0", count3)
	}
}

func TestConvergenceIndex_MaintainedOnStateTransitions(t *testing.T) {
	store := beads.NewMemStore()
	adapter := newConvergenceStoreAdapter(store, nil)

	// Start with an empty index.
	adapter.activeIndex = make(map[string]string)

	// Create a bead and transition through states.
	b, _ := store.Create(beads.Bead{Title: "test", Type: "convergence", Status: "in_progress"})
	_ = store.SetMetadata(b.ID, convergence.FieldTarget, "agent-x")

	// Setting state=active should add to index.
	_ = adapter.SetMetadata(b.ID, convergence.FieldState, convergence.StateActive)
	if _, ok := adapter.activeIndex[b.ID]; !ok {
		t.Error("bead should be in index after state=active")
	}

	// Setting state=terminated should remove from index.
	_ = adapter.SetMetadata(b.ID, convergence.FieldState, convergence.StateTerminated)
	if _, ok := adapter.activeIndex[b.ID]; ok {
		t.Error("bead should not be in index after state=terminated")
	}

	// Setting state=waiting_manual should add to index.
	_ = adapter.SetMetadata(b.ID, convergence.FieldState, convergence.StateWaitingManual)
	if _, ok := adapter.activeIndex[b.ID]; !ok {
		t.Error("bead should be in index after state=waiting_manual")
	}

	// CloseBead should remove from index.
	_ = adapter.CloseBead(b.ID)
	if _, ok := adapter.activeIndex[b.ID]; ok {
		t.Error("bead should not be in index after CloseBead")
	}
}
