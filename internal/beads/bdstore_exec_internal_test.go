//go:build !windows

package beads

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	otellog "go.opentelemetry.io/otel/log"
	otellogglobal "go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// TestExecCommandRunnerTimesOut verifies the runner returns a "timed
// out" error when the command exceeds bdCommandTimeout. No race: we
// only check the error path, not what the child did.
func TestExecCommandRunnerTimesOut(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep unavailable")
	}

	oldTimeout := bdCommandTimeout
	bdCommandTimeout = 3 * time.Second
	t.Cleanup(func() { bdCommandTimeout = oldTimeout })

	_, err := ExecCommandRunner()(t.TempDir(), "sleep", "30")
	if err == nil {
		t.Fatal("runner unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("error = %v, want timeout", err)
	}
}

func TestBDCommandTimeoutForReadCommands(t *testing.T) {
	if got := bdCommandTimeoutFor("bd", []string{"list", "--json"}); got != bdReadCommandTimeout {
		t.Fatalf("bd list timeout = %s, want %s", got, bdReadCommandTimeout)
	}
	if got := bdCommandTimeoutFor("bd", []string{"ready", "--json"}); got != bdReadCommandTimeout {
		t.Fatalf("bd ready timeout = %s, want %s", got, bdReadCommandTimeout)
	}
	if got := bdCommandTimeoutFor("bd", []string{"dep", "list", "gc-1"}); got != bdReadCommandTimeout {
		t.Fatalf("bd dep list timeout = %s, want %s", got, bdReadCommandTimeout)
	}
	if got := bdCommandTimeoutFor("bd", []string{"dep", "add", "gc-1", "gc-2"}); got != bdCommandTimeout {
		t.Fatalf("bd dep add timeout = %s, want %s", got, bdCommandTimeout)
	}
	if got := bdCommandTimeoutFor("bd", []string{"update", "gc-1", "--status", "open"}); got != bdCommandTimeout {
		t.Fatalf("bd update timeout = %s, want %s", got, bdCommandTimeout)
	}
	if got := bdCommandTimeoutFor("git", []string{"status"}); got != bdCommandTimeout {
		t.Fatalf("non-bd timeout = %s, want %s", got, bdCommandTimeout)
	}
}

func TestIsRetryableReadAllowList(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "list", args: []string{"list"}, want: true},
		{name: "show", args: []string{"show", "gc-1"}, want: true},
		{name: "ready", args: []string{"ready"}, want: true},
		{name: "stats", args: []string{"stats"}, want: true},
		{name: "dep list", args: []string{"dep", "list", "gc-1"}, want: true},
		{name: "dep add", args: []string{"dep", "add", "gc-1", "gc-2"}, want: false},
		{name: "update", args: []string{"update", "gc-1"}, want: false},
		{name: "count not retryable", args: []string{"count"}, want: false},
		{name: "empty", args: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableRead(tt.args); got != tt.want {
				t.Fatalf("isRetryableRead(%#v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestExecCommandRunnerRetriesReadTimeoutOnce(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh unavailable")
	}

	oldReadTimeout := bdReadCommandTimeout
	oldRetryTimeout := bdRetryReadTimeout
	bdReadCommandTimeout = 50 * time.Millisecond
	bdRetryReadTimeout = time.Second
	t.Cleanup(func() {
		bdReadCommandTimeout = oldReadTimeout
		bdRetryReadTimeout = oldRetryTimeout
	})

	binDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "attempts")
	writeExecutable(t, filepath.Join(binDir, "bd"), `#!/bin/sh
count=0
if [ -f "$BD_ATTEMPTS_FILE" ]; then
	count=$(cat "$BD_ATTEMPTS_FILE")
fi
count=$((count + 1))
printf '%s' "$count" > "$BD_ATTEMPTS_FILE"
if [ "$count" -eq 1 ]; then
	sleep 1
fi
printf 'ok\n'
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BD_ATTEMPTS_FILE", statePath)

	out, err := ExecCommandRunner()(t.TempDir(), "bd", "list")
	if err != nil {
		t.Fatalf("ExecCommandRunner bd list: %v", err)
	}
	if got := string(out); got != "ok\n" {
		t.Fatalf("stdout = %q, want ok", got)
	}
	if got := readAttemptCount(t, statePath); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
}

func TestExecCommandRunnerDoesNotRetryWriteTimeout(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh unavailable")
	}

	oldTimeout := bdCommandTimeout
	bdCommandTimeout = 50 * time.Millisecond
	t.Cleanup(func() { bdCommandTimeout = oldTimeout })

	binDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "attempts")
	writeExecutable(t, filepath.Join(binDir, "bd"), `#!/bin/sh
count=0
if [ -f "$BD_ATTEMPTS_FILE" ]; then
	count=$(cat "$BD_ATTEMPTS_FILE")
fi
count=$((count + 1))
printf '%s' "$count" > "$BD_ATTEMPTS_FILE"
sleep 1
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BD_ATTEMPTS_FILE", statePath)

	_, err := ExecCommandRunner()(t.TempDir(), "bd", "update", "gc-1")
	if err == nil {
		t.Fatal("ExecCommandRunner unexpectedly succeeded")
	}
	if got := err.Error(); !strings.Contains(got, "bd update timed out: deadline exceeded after 50ms") ||
		!strings.Contains(got, "This was a write operation; no automatic retry was attempted") ||
		!strings.Contains(got, `args=["update" "gc-1"]`) {
		t.Fatalf("error = %q, want no-retry timeout detail", got)
	}
	if got := readAttemptCount(t, statePath); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
}

func TestExecCommandRunnerRetriedReadTimeoutFailureMessage(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh unavailable")
	}

	oldReadTimeout := bdReadCommandTimeout
	oldRetryTimeout := bdRetryReadTimeout
	bdReadCommandTimeout = 40 * time.Millisecond
	bdRetryReadTimeout = 60 * time.Millisecond
	t.Cleanup(func() {
		bdReadCommandTimeout = oldReadTimeout
		bdRetryReadTimeout = oldRetryTimeout
	})

	binDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "attempts")
	writeExecutable(t, filepath.Join(binDir, "bd"), `#!/bin/sh
count=0
if [ -f "$BD_ATTEMPTS_FILE" ]; then
	count=$(cat "$BD_ATTEMPTS_FILE")
fi
count=$((count + 1))
printf '%s' "$count" > "$BD_ATTEMPTS_FILE"
sleep 1
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BD_ATTEMPTS_FILE", statePath)

	_, err := ExecCommandRunner()(t.TempDir(), "bd", "list")
	if err == nil {
		t.Fatal("ExecCommandRunner unexpectedly succeeded")
	}
	if got := err.Error(); !strings.Contains(got, "bd list timed out: deadline exceeded after 40ms, retried with 60ms budget, retry also exceeded") ||
		!strings.Contains(got, "Total elapsed: 100ms") ||
		!strings.Contains(got, `args=["list"]`) {
		t.Fatalf("error = %q, want retried timeout detail", got)
	}
	if got := readAttemptCount(t, statePath); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
}

func TestExecCommandRunnerRetriesTransientSQLErrorsOnReads(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh unavailable")
	}

	tests := []struct {
		name       string
		stderr     string
		wantReason string
	}{
		{
			name:       "deadlock",
			stderr:     "ERROR 1213 (40001): Deadlock found when trying to get lock",
			wantReason: "sql-deadlock",
		},
		{
			name:       "lock wait timeout",
			stderr:     "ERROR 1205 (HY000): Lock wait timeout exceeded",
			wantReason: "sql-lock-wait-timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := installBeadsRecordingLogExporter(t)
			binDir := t.TempDir()
			statePath := filepath.Join(t.TempDir(), "attempts")
			writeExecutable(t, filepath.Join(binDir, "bd"), `#!/bin/sh
count=0
if [ -f "$BD_ATTEMPTS_FILE" ]; then
	count=$(cat "$BD_ATTEMPTS_FILE")
fi
count=$((count + 1))
printf '%s' "$count" > "$BD_ATTEMPTS_FILE"
if [ "$count" -eq 1 ]; then
	printf '%s\n' "$BD_TRANSIENT_STDERR" >&2
	exit 1
fi
printf 'ok\n'
`)
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("BD_ATTEMPTS_FILE", statePath)
			t.Setenv("BD_TRANSIENT_STDERR", tt.stderr)
			t.Setenv("GC_ALIAS", "test-agent-1")

			out, err := ExecCommandRunner()(t.TempDir(), "bd", "list", "--token", "sk-secret")
			if err != nil {
				t.Fatalf("ExecCommandRunner bd list: %v", err)
			}
			if got := string(out); got != "ok\n" {
				t.Fatalf("stdout = %q, want ok", got)
			}
			if got := readAttemptCount(t, statePath); got != 2 {
				t.Fatalf("attempts = %d, want 2", got)
			}
			rec := exp.recordByBody("bd.retry")
			if rec == nil {
				t.Fatal("bd.retry was not emitted")
			}
			attrs := beadsRecordAttrs(*rec)
			if got := attrs["reason"].AsString(); got != tt.wantReason {
				t.Fatalf("bd.retry reason = %q, want %q", got, tt.wantReason)
			}
			if got := attrs["agent_id"].AsString(); got != "test-agent-1" {
				t.Fatalf("bd.retry agent_id = %q, want test-agent-1", got)
			}
			if got := beadsLogValueStringSlice(attrs["args"]); strings.Join(got, " ") != "list --token <redacted>" {
				t.Fatalf("bd.retry args = %#v, want token redacted", got)
			}
		})
	}
}

func TestExecCommandRunnerDoesNotRetryTransientSQLErrorOnWrites(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh unavailable")
	}

	exp := installBeadsRecordingLogExporter(t)
	binDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "attempts")
	writeExecutable(t, filepath.Join(binDir, "bd"), `#!/bin/sh
count=0
if [ -f "$BD_ATTEMPTS_FILE" ]; then
	count=$(cat "$BD_ATTEMPTS_FILE")
fi
count=$((count + 1))
printf '%s' "$count" > "$BD_ATTEMPTS_FILE"
printf 'ERROR 1213 (40001): Deadlock found\n' >&2
exit 1
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BD_ATTEMPTS_FILE", statePath)

	_, err := ExecCommandRunner()(t.TempDir(), "bd", "update", "gc-1")
	if err == nil {
		t.Fatal("ExecCommandRunner unexpectedly succeeded")
	}
	if got := readAttemptCount(t, statePath); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
	if got := exp.countByBody("bd.retry"); got != 0 {
		t.Fatalf("bd.retry records = %d, want 0", got)
	}
}

func TestExecCommandRunnerEmitsBDSlowForLongBDCommand(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh unavailable")
	}

	oldThreshold := bdSlowTelemetryThreshold
	bdSlowTelemetryThreshold = 20 * time.Millisecond
	t.Cleanup(func() { bdSlowTelemetryThreshold = oldThreshold })

	exp := installBeadsRecordingLogExporter(t)
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "bd"), `#!/bin/sh
sleep 0.08
printf '[]\n'
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GC_ALIAS", "test-agent-1")

	if _, err := ExecCommandRunner()(t.TempDir(), "bd", "list", "--token", "sk-secret"); err != nil {
		t.Fatalf("ExecCommandRunner bd: %v", err)
	}

	rec := exp.waitForBody(t, "bd.slow", time.Second)
	attrs := beadsRecordAttrs(*rec)
	if got := beadsLogValueStringSlice(attrs["args"]); strings.Join(got, " ") != "list --token <redacted>" {
		t.Fatalf("bd.slow args = %#v, want token redacted", got)
	}
	if got := attrs["agent_id"].AsString(); got != "test-agent-1" {
		t.Fatalf("bd.slow agent_id = %q, want test-agent-1", got)
	}
}

func TestExecCommandRunnerStopsBDSlowTimerForFastBDCommand(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh unavailable")
	}

	oldThreshold := bdSlowTelemetryThreshold
	bdSlowTelemetryThreshold = 30 * time.Millisecond
	t.Cleanup(func() { bdSlowTelemetryThreshold = oldThreshold })

	exp := installBeadsRecordingLogExporter(t)
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "bd"), `#!/bin/sh
printf '[]\n'
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, err := ExecCommandRunner()(t.TempDir(), "bd", "list"); err != nil {
		t.Fatalf("ExecCommandRunner bd: %v", err)
	}
	time.Sleep(2 * bdSlowTelemetryThreshold)
	if got := exp.countByBody("bd.slow"); got != 0 {
		t.Fatalf("bd.slow records = %d, want 0 for fast bd command", got)
	}
}

// TestKillCommandTreeKillsProcessGroup verifies killCommandTree kills
// the entire process group, not just the direct child. The script
// backgrounds a `sleep 30`; without process-group cleanup, that sleep
// would survive its parent shell's death and leak — the failure mode
// PR #1639 ("kill bd subprocess trees on timeout") fixed.
//
// No timeout involved — we wait synchronously for the script to fork
// the sleep, then call killCommandTree directly. The previous version
// of this test (TestExecCommandRunnerTimeoutKillsChildProcess) raced
// the same assertion against a 50ms timeout, which lost on macOS where
// first-exec of a new script file pays a ~150ms validation tax.
func TestKillCommandTreeKillsProcessGroup(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh unavailable")
	}

	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	script := filepath.Join(dir, "spawn-child.sh")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
sleep 30 &
echo "$!" > "$1"
wait
`), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	cmd := exec.Command(script, pidFile)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		_ = killCommandTree(cmd)
		_ = cmd.Wait()
	})

	childPid := waitForNonEmptyFile(t, pidFile, 5*time.Second)

	if err := killCommandTree(cmd); err != nil {
		t.Fatalf("killCommandTree: %v", err)
	}

	for range 50 {
		if err := exec.Command("kill", "-0", childPid).Run(); err != nil {
			return // child is gone
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = exec.Command("kill", "-KILL", childPid).Run()
	t.Fatalf("child process %s survived killCommandTree", childPid)
}

func TestKillCommandTreeHandlesNilCommand(t *testing.T) {
	if err := killCommandTree(nil); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("killCommandTree(nil): %v", err)
	}
}

func waitForNonEmptyFile(t *testing.T, path string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pidBytes, err := os.ReadFile(path)
		if err == nil {
			pid := strings.TrimSpace(string(pidBytes))
			if pid != "" {
				return pid
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read child pid: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("child pid was not written within %s", timeout)
	return ""
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func readAttemptCount(t *testing.T, path string) int {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read attempts file: %v", err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("parse attempts file %q: %v", raw, err)
	}
	return count
}

type beadsRecordingLogExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func installBeadsRecordingLogExporter(t *testing.T) *beadsRecordingLogExporter {
	t.Helper()
	exp := &beadsRecordingLogExporter{}
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exp)))
	prev := otellogglobal.GetLoggerProvider()
	otellogglobal.SetLoggerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otellogglobal.SetLoggerProvider(prev)
	})
	return exp
}

func (e *beadsRecordingLogExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, rec := range records {
		e.records = append(e.records, rec.Clone())
	}
	return nil
}

func (e *beadsRecordingLogExporter) Shutdown(context.Context) error {
	return nil
}

func (e *beadsRecordingLogExporter) ForceFlush(context.Context) error {
	return nil
}

func (e *beadsRecordingLogExporter) waitForBody(t *testing.T, body string, timeout time.Duration) *sdklog.Record {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if rec := e.recordByBody(body); rec != nil {
			return rec
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("log body %q did not arrive within %s", body, timeout)
	return nil
}

func (e *beadsRecordingLogExporter) recordByBody(body string) *sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.records {
		if e.records[i].Body().AsString() == body {
			rec := e.records[i].Clone()
			return &rec
		}
	}
	return nil
}

func (e *beadsRecordingLogExporter) countByBody(body string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	var count int
	for i := range e.records {
		if e.records[i].Body().AsString() == body {
			count++
		}
	}
	return count
}

func beadsRecordAttrs(rec sdklog.Record) map[string]otellog.Value {
	attrs := make(map[string]otellog.Value)
	rec.WalkAttributes(func(kv otellog.KeyValue) bool {
		attrs[kv.Key] = kv.Value
		return true
	})
	return attrs
}

func beadsLogValueStringSlice(value otellog.Value) []string {
	values := value.AsSlice()
	out := make([]string, 0, len(values))
	for _, item := range values {
		out = append(out, item.AsString())
	}
	return out
}
