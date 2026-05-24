package main

import (
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
)

// TestDoSlingNudgePoolNoRunningInstanceEnqueuesQueuedNudge verifies that when
// doSlingNudge runs for a pool agent (SupportsInstanceExpansion=true) with no
// running instances, it enqueues a queued nudge so the agent wakes to the
// pending sling when it next starts. This is parity with the fixed-agent path
// (deliverSlingNudge always enqueues for asleep targets).
//
// This test FAILS on current code: the pool no-running-instance branch only
// calls pokeController and returns without calling enqueueQueuedNudge.
func TestDoSlingNudgePoolNoRunningInstanceEnqueuesQueuedNudge(t *testing.T) {
	t.Skip("ga-3dxxs.1: remove Skip and implement enqueueQueuedNudge call in doSlingNudge pool no-running-instance branch")
	runner := newFakeRunner()
	sp := runtime.NewFake()
	// No pool instances running — poke will fail (no controller socket in test).
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{
		Name:              "polecat",
		Dir:               "hw",
		MinActiveSessions: intPtr(1), MaxActiveSessions: intPtr(3),
	}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	deps.CityPath = t.TempDir()
	opts := testOpts(a, "BL-1")
	opts.Nudge = true
	code := doSling(opts, deps, nil, stdout, stderr)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	pending, _, dead, err := listQueuedNudges(deps.CityPath, a.QualifiedName(), time.Now())
	if err != nil {
		t.Fatalf("listQueuedNudges: %v", err)
	}
	if len(pending) != 1 || len(dead) != 0 {
		t.Fatalf("pending=%d dead=%d, want 1/0 — pool nudge with no running instance must enqueue a queued nudge", len(pending), len(dead))
	}
	if pending[0].Source != "sling" {
		t.Errorf("source = %q, want sling", pending[0].Source)
	}
	if pending[0].Agent != a.QualifiedName() {
		t.Errorf("agent = %q, want %q", pending[0].Agent, a.QualifiedName())
	}
}
