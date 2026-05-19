package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/user"
	"sort"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/convergence"
)

const cityConvergenceStoreKey = ""

// convergenceRequest is a command sent from the controller socket to the
// event loop for serialized processing.
type convergenceRequest struct {
	Command string            `json:"command"` // create, approve, iterate, stop, retry
	BeadID  string            `json:"bead_id"`
	User    string            `json:"user,omitempty"` // resolved client-side for audit attribution
	Params  map[string]string `json:"params"`         // command-specific parameters
	replyCh chan convergenceReply
}

// convergenceReply is the response from the event loop to a socket command.
type convergenceReply struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// initConvergenceHandlers creates convergence handlers for the city store and
// configured rig stores. It is called during CityRuntime.run() initialization
// and again on config reload.
func (cr *CityRuntime) initConvergenceHandlers() {
	cr.convStoreAdapters = make(map[string]*convergenceStoreAdapter)
	cr.convHandlers = make(map[string]*convergence.Handler)

	if cr.cfg == nil {
		return
	}
	if store := cr.cityBeadStore(); store != nil {
		if err := cr.wireConvergenceHandler(cityConvergenceStoreKey, store, cr.cfg.FormulaLayers.City); err != nil {
			cr.logConvergenceInitError("city", err)
		}
	}
	for _, rig := range cr.cfg.Rigs {
		store, err := cr.rigBeadStore(rig)
		if err != nil {
			cr.logConvergenceInitError(fmt.Sprintf("rig %q", rig.Name), err)
			continue
		}
		if err := cr.wireConvergenceHandler(rig.Name, store, cr.cfg.FormulaLayers.SearchPaths(rig.Name)); err != nil {
			cr.logConvergenceInitError(fmt.Sprintf("rig %q", rig.Name), err)
		}
	}
}

func (cr *CityRuntime) wireConvergenceHandler(key string, store beads.Store, formulaSearchPaths []string) error {
	if store == nil {
		return fmt.Errorf("bead store is nil")
	}
	if key != cityConvergenceStoreKey {
		if err := store.Ping(); err != nil {
			return err
		}
	}
	adapter := newConvergenceStoreAdapter(store, formulaSearchPaths)
	emitter := &convergenceEventEmitter{rec: cr.rec}
	cr.convStoreAdapters[key] = adapter
	cr.convHandlers[key] = &convergence.Handler{
		Store:   adapter,
		Emitter: emitter,
	}
	return nil
}

func (cr *CityRuntime) logConvergenceInitError(scope string, err error) {
	if cr.stderr == nil {
		return
	}
	fmt.Fprintf(cr.stderr, "%s: convergence: %s unavailable: %v\n", cr.logPrefix, scope, err) //nolint:errcheck
}

func (cr *CityRuntime) cityConvergenceHandler() *convergence.Handler {
	if cr.convHandlers == nil {
		return nil
	}
	return cr.convHandlers[cityConvergenceStoreKey]
}

func (cr *CityRuntime) cityConvergenceStoreAdapter() *convergenceStoreAdapter {
	if cr.convStoreAdapters == nil {
		return nil
	}
	return cr.convStoreAdapters[cityConvergenceStoreKey]
}

func (cr *CityRuntime) hasConvergenceHandlers() bool {
	return len(cr.convHandlers) > 0
}

func (cr *CityRuntime) convergenceStoreKeysCityFirst() []string {
	if len(cr.convHandlers) == 0 {
		return nil
	}
	keys := make([]string, 0, len(cr.convHandlers))
	if _, ok := cr.convHandlers[cityConvergenceStoreKey]; ok {
		keys = append(keys, cityConvergenceStoreKey)
	}
	rigKeys := make([]string, 0, len(cr.convHandlers))
	for key := range cr.convHandlers {
		if key == cityConvergenceStoreKey {
			continue
		}
		rigKeys = append(rigKeys, key)
	}
	sort.Strings(rigKeys)
	keys = append(keys, rigKeys...)
	return keys
}

func (cr *CityRuntime) convergenceHandlerForBead(beadID string) (*convergence.Handler, error) {
	for _, key := range cr.convergenceStoreKeysCityFirst() {
		handler := cr.convHandlers[key]
		if handler == nil {
			continue
		}
		if _, err := handler.Store.GetMetadata(beadID); err == nil {
			return handler, nil
		} else if !errors.Is(err, beads.ErrNotFound) {
			return nil, fmt.Errorf("reading convergence bead %q in store %q: %w", beadID, key, err)
		}
	}
	return nil, fmt.Errorf("convergence bead %q not found", beadID)
}

func unavailableConvergenceRigError(rigName string) string {
	return fmt.Sprintf("rig %q is not available; check that it is configured and that its store is reachable", rigName)
}

// convergenceTick processes active convergence loops by checking indexed
// beads for closed wisps and calling HandleWispClosed. Called from tick().
// Uses the in-memory active index (O(active) instead of O(all beads)).
func (cr *CityRuntime) convergenceTick(ctx context.Context) {
	if !cr.hasConvergenceHandlers() || cr.convergenceReqCh == nil {
		return
	}
	for _, key := range cr.convergenceStoreKeysCityFirst() {
		handler := cr.convHandlers[key]
		adapter := cr.convStoreAdapters[key]
		if handler == nil || adapter == nil || adapter.activeIndex == nil {
			continue
		}
		for _, beadID := range adapter.activeBeadIDs() {
			meta, err := adapter.GetMetadata(beadID)
			if err != nil {
				continue
			}
			// Only process active beads; skip others like waiting_manual
			// that are indexed for CountActiveConvergenceLoops but not for tick.
			if meta[convergence.FieldState] != convergence.StateActive {
				continue
			}
			activeWisp := meta[convergence.FieldActiveWisp]
			if activeWisp == "" {
				continue
			}
			// Check if the active wisp is closed.
			wispInfo, wErr := adapter.GetBead(activeWisp)
			if wErr != nil {
				continue // wisp may not exist yet
			}
			if wispInfo.Status != "closed" {
				continue
			}
			// Process the closed wisp.
			result, hErr := handler.HandleWispClosed(ctx, beadID, activeWisp)
			if hErr != nil {
				fmt.Fprintf(cr.stderr, "%s: convergence: HandleWispClosed(%s, %s): %v\n", //nolint:errcheck
					cr.logPrefix, beadID, activeWisp, hErr)
				continue
			}
			if result.Action != convergence.ActionSkipped {
				fmt.Fprintf(cr.stdout, "Convergence %s: %s (iteration %d)\n", //nolint:errcheck
					beadID, result.Action, result.Iteration)
			}
		}
	}
}

// processConvergenceRequests drains the convergence request channel and
// processes each command serially. Called from the event loop to serialize
// CLI commands with tick-based processing.
func (cr *CityRuntime) processConvergenceRequests(ctx context.Context) {
	if !cr.hasConvergenceHandlers() || cr.convergenceReqCh == nil {
		return
	}
	for {
		select {
		case req := <-cr.convergenceReqCh:
			reply := cr.safeHandleConvergenceRequest(ctx, req)
			req.replyCh <- reply
		default:
			return
		}
	}
}

// safeHandleConvergenceRequest wraps handleConvergenceRequest with panic
// recovery so a panicking handler doesn't leave replyCh unwritten and hang
// the socket handler goroutine.
func (cr *CityRuntime) safeHandleConvergenceRequest(ctx context.Context, req convergenceRequest) (reply convergenceReply) {
	defer func() {
		if r := recover(); r != nil {
			reply = convergenceReply{Error: fmt.Sprintf("internal error (panic): %v", r)}
			fmt.Fprintf(cr.stderr, "%s: convergence: panic handling %q for %s: %v\n", //nolint:errcheck
				cr.logPrefix, req.Command, req.BeadID, r)
		}
	}()
	reply = cr.handleConvergenceRequest(ctx, req)
	if reply.Error != "" {
		fmt.Fprintf(cr.stderr, "%s: convergence: %s %s: %s\n", //nolint:errcheck
			cr.logPrefix, req.Command, req.BeadID, reply.Error)
	}
	return reply
}

// handleConvergenceRequest dispatches a single convergence command.
func (cr *CityRuntime) handleConvergenceRequest(ctx context.Context, req convergenceRequest) convergenceReply {
	if !cr.hasConvergenceHandlers() {
		return convergenceReply{Error: "convergence not available (no bead store)"}
	}

	// Use client-supplied username for audit attribution; fall back to
	// daemon user only if the client didn't provide one.
	username := req.User
	if username == "" {
		username = currentUsername()
	}

	switch req.Command {
	case "create":
		return cr.handleConvergenceCreate(ctx, req)
	case "approve":
		handler, err := cr.convergenceHandlerForBead(req.BeadID)
		if err != nil {
			return convergenceReply{Error: err.Error()}
		}
		result, err := handler.ApproveHandler(ctx, req.BeadID, username, "")
		if err != nil {
			return convergenceReply{Error: err.Error()}
		}
		return marshalReply(result)
	case "iterate":
		handler, err := cr.convergenceHandlerForBead(req.BeadID)
		if err != nil {
			return convergenceReply{Error: err.Error()}
		}
		result, err := handler.IterateHandler(ctx, req.BeadID, username, "")
		if err != nil {
			return convergenceReply{Error: err.Error()}
		}
		return marshalReply(result)
	case "stop":
		handler, err := cr.convergenceHandlerForBead(req.BeadID)
		if err != nil {
			return convergenceReply{Error: err.Error()}
		}
		result, err := handler.StopHandler(ctx, req.BeadID, username, "")
		if err != nil {
			return convergenceReply{Error: err.Error()}
		}
		return marshalReply(result)
	case "retry":
		return cr.handleConvergenceRetry(ctx, req)
	default:
		return convergenceReply{Error: fmt.Sprintf("unknown convergence command: %q", req.Command)}
	}
}

// handleConvergenceCreate processes a create command.
func (cr *CityRuntime) handleConvergenceCreate(ctx context.Context, req convergenceRequest) convergenceReply {
	rigName := req.Params["rig"]
	handler := cr.convHandlers[rigName]
	if handler == nil {
		if rigName != "" {
			return convergenceReply{Error: unavailableConvergenceRigError(rigName)}
		}
		return convergenceReply{Error: "convergence not available (no bead store)"}
	}

	formula := req.Params["formula"]
	target := req.Params["target"]
	maxIter := 5
	if v, ok := convergence.DecodeInt(req.Params["max_iterations"]); ok && v > 0 {
		maxIter = v
	}

	gateMode := req.Params["gate_mode"]
	if gateMode == "" {
		gateMode = convergence.GateModeManual
	}

	// Concurrency checks.
	maxPerAgent := cr.cfg.Convergence.MaxPerAgentOrDefault()
	if err := convergence.CheckConcurrencyLimits(handler.Store, target, maxPerAgent); err != nil {
		return convergenceReply{Error: err.Error()}
	}
	if err := convergence.CheckNestedConvergence(handler.Store, "", target); err != nil {
		return convergenceReply{Error: err.Error()}
	}

	// Build vars from params with "var." prefix.
	vars := make(map[string]string)
	for k, v := range req.Params {
		if len(k) > 4 && k[:4] == "var." {
			vars[k[4:]] = v
		}
	}

	params := convergence.CreateParams{
		Formula:           formula,
		Target:            target,
		MaxIterations:     maxIter,
		GateMode:          gateMode,
		GateCondition:     req.Params["gate_condition"],
		GateTimeout:       req.Params["gate_timeout"],
		GateTimeoutAction: req.Params["gate_timeout_action"],
		Title:             req.Params["title"],
		Vars:              vars,
		CityPath:          cr.cityPath,
		EvaluatePrompt:    req.Params["evaluate_prompt"],
	}

	result, err := handler.CreateHandler(ctx, params)
	if err != nil {
		return convergenceReply{Error: err.Error()}
	}
	return marshalReply(result)
}

// handleConvergenceRetry processes a retry command.
func (cr *CityRuntime) handleConvergenceRetry(ctx context.Context, req convergenceRequest) convergenceReply {
	handler, findErr := cr.convergenceHandlerForBead(req.BeadID)
	if findErr != nil {
		return convergenceReply{Error: findErr.Error()}
	}

	sourceBeadID := req.BeadID
	maxIter := 0
	if v, ok := convergence.DecodeInt(req.Params["max_iterations"]); ok && v > 0 {
		maxIter = v
	}

	// Read source bead metadata once for both max_iterations and target.
	meta, err := handler.Store.GetMetadata(sourceBeadID)
	if err != nil {
		return convergenceReply{Error: fmt.Sprintf("reading source bead: %v", err)}
	}

	// If no max_iterations specified, read from source bead.
	if maxIter == 0 {
		if v, ok := convergence.DecodeInt(meta[convergence.FieldMaxIterations]); ok {
			maxIter = v
		}
		if maxIter == 0 {
			maxIter = 5
		}
	}

	target := meta[convergence.FieldTarget]

	// Concurrency checks.
	maxPerAgent := cr.cfg.Convergence.MaxPerAgentOrDefault()
	if err := convergence.CheckConcurrencyLimits(handler.Store, target, maxPerAgent); err != nil {
		return convergenceReply{Error: err.Error()}
	}
	if err := convergence.CheckNestedConvergence(handler.Store, "", target); err != nil {
		return convergenceReply{Error: err.Error()}
	}

	username := req.User
	if username == "" {
		username = currentUsername()
	}

	result, err := handler.RetryHandler(ctx, sourceBeadID, username, maxIter)
	if err != nil {
		return convergenceReply{Error: err.Error()}
	}
	return marshalReply(result)
}

// convergenceStartupReconcile runs convergence bead reconciliation on startup
// and then populates the in-memory active index.
func (cr *CityRuntime) convergenceStartupReconcile(ctx context.Context) {
	if !cr.hasConvergenceHandlers() || cr.convergenceReqCh == nil {
		return
	}

	for _, key := range cr.convergenceStoreKeysCityFirst() {
		handler := cr.convHandlers[key]
		adapter := cr.convStoreAdapters[key]
		if handler == nil || adapter == nil {
			continue
		}

		all, err := adapter.store.List()
		if err != nil {
			fmt.Fprintf(cr.stderr, "%s: convergence reconcile: listing beads: %v\n", cr.logPrefix, err) //nolint:errcheck
			continue
		}

		var beadIDs []string
		for _, b := range all {
			if b.Type == "convergence" && b.Status != "closed" {
				beadIDs = append(beadIDs, b.ID)
			}
		}

		if len(beadIDs) > 0 {
			reconciler := &convergence.Reconciler{Handler: handler}
			report, err := reconciler.ReconcileBeads(ctx, beadIDs)
			if err != nil {
				fmt.Fprintf(cr.stderr, "%s: convergence reconciliation: %v\n", cr.logPrefix, err) //nolint:errcheck
				continue
			}
			if report.Recovered > 0 || report.Errors > 0 {
				fmt.Fprintf(cr.stdout, "Convergence recovery: %d scanned, %d recovered, %d errors\n", //nolint:errcheck
					report.Scanned, report.Recovered, report.Errors)
			}
		}

		// Populate the active index after reconciliation so it reflects
		// post-recovery state.
		if err := adapter.populateIndex(); err != nil {
			fmt.Fprintf(cr.stderr, "%s: convergence: populating active index: %v\n", cr.logPrefix, err) //nolint:errcheck
		}
	}
}

// sendConvergenceRequest sends a request through the controller socket and
// waits for a reply. Used by CLI commands.
func sendConvergenceRequest(cityPath string, req convergenceRequest) (convergenceReply, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return convergenceReply{}, fmt.Errorf("marshaling request: %w", err)
	}
	respBytes, err := sendControllerCommand(cityPath, "converge:"+string(data))
	if err != nil {
		return convergenceReply{}, err
	}
	var reply convergenceReply
	if err := json.Unmarshal(respBytes, &reply); err != nil {
		return convergenceReply{}, fmt.Errorf("parsing response: %w", err)
	}
	return reply, nil
}

func marshalReply(v any) convergenceReply {
	data, err := json.Marshal(v)
	if err != nil {
		return convergenceReply{Error: fmt.Sprintf("marshaling result: %v", err)}
	}
	return convergenceReply{Result: data}
}

func currentUsername() string {
	u, err := user.Current()
	if err != nil {
		return "unknown"
	}
	return u.Username
}
