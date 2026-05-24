//go:build ignore

// Remove the line above when Scorecard.HostOverloaded and Runner.HostLoadRatioFn are implemented (ga-bctco.1).

package coordstore_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/benchmarks/coordstore"
	"github.com/gastownhall/gascity/internal/benchmarks/coordstore/adapters/hqstore"
)

// latencyOnlyFailResult returns a ScorecardResult for a p99 latency target that
// has been measured and failed, with no throughput component.
func latencyOnlyFailResult() coordstore.ScorecardResult {
	return coordstore.ScorecardResult{
		Target:    coordstore.Target{Op: "Get", Name: "point read (FR-3)", P99: 1 * time.Millisecond},
		Measured:  true,
		Pass:      false,
		Reason:    "p99 10ms > target 1ms",
		ActualP99: 10 * time.Millisecond,
	}
}

// throughputFailResult returns a ScorecardResult for a throughput target that
// has been measured and failed.
func throughputFailResult() coordstore.ScorecardResult {
	return coordstore.ScorecardResult{
		Target:           coordstore.Target{Op: "MailPoll", Name: "mail-poll read throughput", MinThroughput: 150},
		Measured:         true,
		Pass:             false,
		Reason:           "throughput 10/s < target 150/s",
		ActualThroughput: 10,
	}
}

// TestScorecardPassed_HostOverloaded_LatencyOnlyFails_Passes verifies that a
// Scorecard with HostOverloaded=true treats measured-but-failed latency targets
// as passing. Elevated latency under host overload is an expected artifact of
// resource contention, not a backend defect.
//
// This test FAILS TO COMPILE on current code: Scorecard has no HostOverloaded field.
func TestScorecardPassed_HostOverloaded_LatencyOnlyFails_Passes(t *testing.T) {
	sc := coordstore.Scorecard{
		Results: []coordstore.ScorecardResult{latencyOnlyFailResult()},
	}
	sc.HostOverloaded = true // compile error: Scorecard has no field HostOverloaded

	if !sc.Passed() {
		t.Error("Passed() = false, want true — latency failure under host overload must be suppressed")
	}
}

// TestScorecardPassed_HostOverloaded_ThroughputFails_Fails verifies that
// HostOverloaded=true does NOT suppress throughput failures. Sustained
// throughput degradation exceeds what transient host overload explains.
//
// This test FAILS TO COMPILE on current code: Scorecard has no HostOverloaded field.
func TestScorecardPassed_HostOverloaded_ThroughputFails_Fails(t *testing.T) {
	sc := coordstore.Scorecard{
		Results: []coordstore.ScorecardResult{throughputFailResult()},
	}
	sc.HostOverloaded = true // compile error: Scorecard has no field HostOverloaded

	if sc.Passed() {
		t.Error("Passed() = true, want false — throughput failure must not be suppressed by host overload")
	}
}

// TestPrintTable_HostOverloaded_EmitsWarningOnce verifies that PrintTable
// includes a host-overload warning line when HostOverloaded is set. The warning
// must appear exactly once so that log scanners can match it reliably.
//
// This test FAILS TO COMPILE on current code: Scorecard has no HostOverloaded field.
func TestPrintTable_HostOverloaded_EmitsWarningOnce(t *testing.T) {
	sc := coordstore.Scorecard{
		Backend:  "hqstore",
		Workload: "smoke",
		Results:  []coordstore.ScorecardResult{latencyOnlyFailResult()},
	}
	sc.HostOverloaded = true // compile error: Scorecard has no field HostOverloaded

	var buf bytes.Buffer
	sc.PrintTable(&buf)
	output := buf.String()

	count := strings.Count(strings.ToLower(output), "overload")
	if count == 0 {
		t.Errorf("PrintTable output contains no 'overload' warning, want exactly one\noutput:\n%s", output)
	}
	if count > 1 {
		t.Errorf("PrintTable output contains %d 'overload' occurrences, want exactly one\noutput:\n%s", count, output)
	}
}

// TestPrintTable_HostOverloaded_LatencyTargetsSkipped verifies that a failed
// latency target is not reported as FAIL in the table when HostOverloaded is
// set. The table row must not contain "FAIL" for that target so that human
// readers and log parsers do not misinterpret the suppressed result.
//
// This test FAILS TO COMPILE on current code: Scorecard has no HostOverloaded field.
func TestPrintTable_HostOverloaded_LatencyTargetsSkipped(t *testing.T) {
	sc := coordstore.Scorecard{
		Backend:  "hqstore",
		Workload: "smoke",
		Results:  []coordstore.ScorecardResult{latencyOnlyFailResult()},
	}
	sc.HostOverloaded = true // compile error: Scorecard has no field HostOverloaded

	var buf bytes.Buffer
	sc.PrintTable(&buf)
	output := buf.String()

	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "point read") && strings.Contains(line, "FAIL") {
			t.Errorf("latency target row contains FAIL under host overload, want suppressed:\n  %s", line)
		}
	}
}

// TestRunnerSetsHostOverloaded_WhenLoadRatioExceedsThreshold verifies that the
// Runner sets HostOverloaded=true on the returned Scorecard when the injected
// HostLoadRatioFn reports a ratio that exceeds the overload threshold.
//
// This test FAILS TO COMPILE on current code: Runner has no HostLoadRatioFn
// field and Scorecard has no HostOverloaded field.
func TestRunnerSetsHostOverloaded_WhenLoadRatioExceedsThreshold(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	adapter := hqstore.New()
	if err := adapter.Open(ctx, coordstore.Config{DataDir: dir}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer adapter.Close() //nolint:errcheck

	wl := coordstore.WorkloadConfig{
		Name:          "overload-unit",
		Duration:      300 * time.Millisecond,
		Concurrency:   1,
		MainOpenCount: 10,
		WispOpenCount: 10,
		PointReadRate: 1.0,
		MailPollRate:  1.0,
	}
	seeder := coordstore.NewSeeder(0xdeadbeef)
	seed, err := seeder.Seed(ctx, adapter, wl)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}

	r := coordstore.NewRunner(adapter, wl, seed)
	r.HostLoadRatioFn = func() float64 { return 0.95 } // compile error: Runner has no field HostLoadRatioFn

	sc, err := r.Run(ctx, io.Discard)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !sc.HostOverloaded { // compile error: Scorecard has no field HostOverloaded
		t.Error("HostOverloaded = false, want true — load ratio 0.95 must exceed overload threshold")
	}
}
