//go:build integration

package integration

// Integration tests for the gc emergency reader CLI (list/show/ack) and gc
// doctor emergency-spool check. These tests drive the real gc binary against
// a minimal city directory — no running controller, no agents — so they stay
// fast while proving the real spool-walk, JSON serialisation, ack semantics,
// and doctor prune path.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/emergency"
)

// setupEmergencyTestCity creates a minimal city directory that gc emergency
// commands can discover via cwd walk-up (city.toml at root).
func setupEmergencyTestCity(t *testing.T) string {
	t.Helper()
	cityDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(cityDir, "city.toml"),
		[]byte("[city]\nname = \"gctest-emergency\"\n"),
		0o644,
	); err != nil {
		t.Fatalf("write city.toml: %v", err)
	}
	return cityDir
}

// extractEmergencyID finds the first line in out that is exactly a valid
// emergency record ID. gc emergency send prints diagnostic messages to stderr
// and the bare ID to stdout; since gc() returns CombinedOutput, we scan for
// the line that is only an ID.
func extractEmergencyID(out string) string {
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if emergency.ValidRecordID(line) {
			return line
		}
	}
	return ""
}

// gcEmergencySend sends one emergency record and returns its ID.
func gcEmergencySend(t *testing.T, cityDir, severity, message string) string {
	t.Helper()
	out, err := gc(cityDir, "emergency", "send", "--quiet", "--severity", severity, message)
	if err != nil {
		t.Fatalf("gc emergency send (severity=%s, msg=%q): %v\noutput: %s", severity, message, err, out)
	}
	id := extractEmergencyID(out)
	if id == "" {
		t.Fatalf("gc emergency send: no valid id in output:\n%s", out)
	}
	return id
}

// TestEmergencyReader_SendListAckShow exercises the full reader flow:
//
//  1. Write 5 records via gc emergency send (mix of severities).
//  2. Verify gc emergency list (default table) shows all 5 unacked.
//  3. Ack 2 records via gc emergency ack; verify stdout "acked: <id>".
//  4. gc emergency list shows only 3 unacked; acked IDs are absent.
//  5. gc emergency list --all shows all 5 with STATUS column.
//  6. gc emergency show for an open record: valid JSON with matching ID.
//  7. gc emergency show for an acked record: valid JSON with matching ID.
//  8. gc emergency list --format json: 3 NDJSON lines (one per open record).
//  9. Idempotent re-ack: exit 0 with "already acked: <id>".
//  10. Malformed id: exit non-zero with "invalid id" message.
func TestEmergencyReader_SendListAckShow(t *testing.T) {
	cityDir := setupEmergencyTestCity(t)

	// 1. Send 5 records with distinct severities and messages.
	type sendCase struct {
		severity string
		message  string
	}
	sends := []sendCase{
		{"critical", "substrate failure: dolt connection refused"},
		{"error", "mail send failed: bd write unavailable"},
		{"warn", "session stall detected: agent idle too long"},
		{"error", "sling failed: molecule step timed out"},
		{"info", "reconciler tick completed with warnings"},
	}
	ids := make([]string, len(sends))
	for i, s := range sends {
		ids[i] = gcEmergencySend(t, cityDir, s.severity, fmt.Sprintf("%s (seq=%d)", s.message, i))
	}

	// 2. gc emergency list — default table shows all 5 unacked.
	out, err := gc(cityDir, "emergency", "list")
	if err != nil {
		t.Fatalf("gc emergency list: %v\noutput: %s", err, out)
	}
	for _, id := range ids {
		if !strings.Contains(out, id) {
			t.Fatalf("gc emergency list: missing id %q in output:\n%s", id, out)
		}
	}
	if !strings.Contains(out, "5 unacked") {
		t.Fatalf("gc emergency list: want '5 unacked' in summary, got:\n%s", out)
	}

	// 3. Ack the first two records.
	for _, id := range ids[:2] {
		out, err := gc(cityDir, "emergency", "ack", id)
		if err != nil {
			t.Fatalf("gc emergency ack %s: %v\noutput: %s", id, err, out)
		}
		if !strings.Contains(out, "acked: "+id) {
			t.Fatalf("gc emergency ack %s: want 'acked: %s' in output:\n%s", id, id, out)
		}
	}

	// 4. gc emergency list (default) — 3 unacked; acked IDs absent.
	out, err = gc(cityDir, "emergency", "list")
	if err != nil {
		t.Fatalf("gc emergency list after ack: %v\noutput: %s", err, out)
	}
	for _, id := range ids[:2] {
		if strings.Contains(out, id) {
			t.Fatalf("gc emergency list: acked id %q must not appear in default list:\n%s", id, out)
		}
	}
	for _, id := range ids[2:] {
		if !strings.Contains(out, id) {
			t.Fatalf("gc emergency list: open id %q missing from default list:\n%s", id, out)
		}
	}
	if !strings.Contains(out, "3 unacked") {
		t.Fatalf("gc emergency list after ack: want '3 unacked' in summary, got:\n%s", out)
	}

	// 5. gc emergency list --all — all 5 entries with STATUS column.
	out, err = gc(cityDir, "emergency", "list", "--all")
	if err != nil {
		t.Fatalf("gc emergency list --all: %v\noutput: %s", err, out)
	}
	for _, id := range ids {
		if !strings.Contains(out, id) {
			t.Fatalf("gc emergency list --all: missing id %q:\n%s", id, out)
		}
	}
	if !strings.Contains(out, "STATUS") {
		t.Fatalf("gc emergency list --all: want STATUS column header:\n%s", out)
	}
	if !strings.Contains(out, "5 entries") {
		t.Fatalf("gc emergency list --all: want '5 entries' in summary:\n%s", out)
	}

	// 6. gc emergency show — open record: JSON with correct ID.
	out, err = gc(cityDir, "emergency", "show", ids[2])
	if err != nil {
		t.Fatalf("gc emergency show %s: %v\noutput: %s", ids[2], err, out)
	}
	var openRec emergency.Record
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &openRec); err != nil {
		t.Fatalf("gc emergency show %s: output is not JSON: %v\noutput: %s", ids[2], err, out)
	}
	if openRec.ID != ids[2] {
		t.Fatalf("gc emergency show %s: record id = %q, want %q", ids[2], openRec.ID, ids[2])
	}

	// 7. gc emergency show — acked record: JSON with correct ID.
	out, err = gc(cityDir, "emergency", "show", ids[0])
	if err != nil {
		t.Fatalf("gc emergency show (acked) %s: %v\noutput: %s", ids[0], err, out)
	}
	var ackedRec emergency.Record
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &ackedRec); err != nil {
		t.Fatalf("gc emergency show (acked) %s: output is not JSON: %v\noutput: %s", ids[0], err, out)
	}
	if ackedRec.ID != ids[0] {
		t.Fatalf("gc emergency show (acked) %s: record id = %q, want %q", ids[0], ackedRec.ID, ids[0])
	}

	// 8. gc emergency list --format json — 3 NDJSON lines (one per open record).
	out, err = gc(cityDir, "emergency", "list", "--format", "json")
	if err != nil {
		t.Fatalf("gc emergency list --format json: %v\noutput: %s", err, out)
	}
	var ndjsonLines []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			ndjsonLines = append(ndjsonLines, line)
		}
	}
	if len(ndjsonLines) != 3 {
		t.Fatalf("gc emergency list --format json: want 3 NDJSON lines (3 open records), got %d:\n%s", len(ndjsonLines), out)
	}
	for i, line := range ndjsonLines {
		var rec emergency.Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("gc emergency list --format json line %d: JSON parse failed: %v\nline: %s", i, err, line)
		}
		if !emergency.ValidRecordID(rec.ID) {
			t.Fatalf("gc emergency list --format json line %d: invalid record id %q", i, rec.ID)
		}
	}

	// 9. Idempotent re-ack: gc emergency ack on already-acked ID exits 0.
	out, err = gc(cityDir, "emergency", "ack", ids[0])
	if err != nil {
		t.Fatalf("gc emergency ack (idempotent) %s: want exit 0, got %v\noutput: %s", ids[0], err, out)
	}
	if !strings.Contains(out, "already acked: "+ids[0]) {
		t.Fatalf("gc emergency ack (idempotent) %s: want 'already acked: %s' in output:\n%s", ids[0], ids[0], out)
	}

	// 10. Malformed id: exit non-zero with "invalid id" in output.
	out, err = gc(cityDir, "emergency", "ack", "../../etc/passwd")
	if err == nil {
		t.Fatalf("gc emergency ack (traversal): want non-zero exit, got nil error\noutput: %s", out)
	}
	if !strings.Contains(out, "invalid id") {
		t.Fatalf("gc emergency ack (traversal): want 'invalid id' in output:\n%s", out)
	}
}

// TestEmergencyReader_HookInjectionFormat verifies that gc emergency list
// --format hook-injection produces the <system-reminder> block when there are
// unacked records, and empty output when the spool is clear.
func TestEmergencyReader_HookInjectionFormat(t *testing.T) {
	cityDir := setupEmergencyTestCity(t)

	// No records: hook-injection should produce empty output (exit 0).
	out, err := gc(cityDir, "emergency", "list", "--format", "hook-injection")
	if err != nil {
		t.Fatalf("gc emergency list --format hook-injection (empty): %v\noutput: %s", err, out)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("gc emergency list --format hook-injection with no records: want empty output, got:\n%s", out)
	}

	// Send 2 unacked records.
	id1 := gcEmergencySend(t, cityDir, "critical", "dolt write failed")
	id2 := gcEmergencySend(t, cityDir, "error", "mail routing error")

	// Hook-injection block should appear and mention the unacked records.
	out, err = gc(cityDir, "emergency", "list", "--format", "hook-injection")
	if err != nil {
		t.Fatalf("gc emergency list --format hook-injection: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "<system-reminder>") {
		t.Fatalf("gc emergency list --format hook-injection: missing <system-reminder>:\n%s", out)
	}
	if !strings.Contains(out, "unacked emergency signal") {
		t.Fatalf("gc emergency list --format hook-injection: missing 'unacked emergency signal':\n%s", out)
	}
	// Both IDs must appear (limit 20 > 2).
	for _, id := range []string{id1, id2} {
		if !strings.Contains(out, id) {
			t.Fatalf("gc emergency list --format hook-injection: missing id %q:\n%s", id, out)
		}
	}

	// Ack both. Re-run hook-injection: must produce empty output.
	for _, id := range []string{id1, id2} {
		if _, err := gc(cityDir, "emergency", "ack", id); err != nil {
			t.Fatalf("gc emergency ack %s: %v", id, err)
		}
	}
	out, err = gc(cityDir, "emergency", "list", "--format", "hook-injection")
	if err != nil {
		t.Fatalf("gc emergency list --format hook-injection (all acked): %v\noutput: %s", err, out)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("gc emergency list --format hook-injection with all acked: want empty output, got:\n%s", out)
	}
}

// TestEmergencyDoctor_ReportsAndFixes verifies the gc doctor emergency-spool
// check: it warns when unacked records exist, and gc doctor --fix with a tiny
// --emergency-ttl prunes processed records while leaving open ones intact.
func TestEmergencyDoctor_ReportsAndFixes(t *testing.T) {
	cityDir := setupEmergencyTestCity(t)

	// Send 3 records.
	ids := make([]string, 3)
	for i := range ids {
		ids[i] = gcEmergencySend(t, cityDir, "error", fmt.Sprintf("doctor integration test record %d", i))
	}

	// Ack the first 2 records; leave ids[2] open.
	for _, id := range ids[:2] {
		out, err := gc(cityDir, "emergency", "ack", id)
		if err != nil {
			t.Fatalf("gc emergency ack %s: %v\noutput: %s", id, err, out)
		}
	}

	// gc doctor (no fix): emergency-spool check should warn about 1 unacked entry.
	// Doctor may also report other warnings/errors for the minimal city, so we
	// do not assert on the overall exit code — only on the emergency-spool line.
	out, _ := gc(cityDir, "doctor")
	if !strings.Contains(out, "emergency-spool") {
		t.Fatalf("gc doctor: want 'emergency-spool' in output:\n%s", out)
	}
	if !strings.Contains(out, "unacked") {
		t.Fatalf("gc doctor: want 'unacked' in emergency-spool output:\n%s", out)
	}

	// gc doctor --fix --emergency-ttl 1ns: prunes all processed records
	// (any age exceeds a 1 ns TTL) while leaving the open record.
	out, _ = gc(cityDir, "doctor", "--fix", "--emergency-ttl", "1ns")
	if !strings.Contains(out, "emergency-spool") {
		t.Fatalf("gc doctor --fix: want 'emergency-spool' in output:\n%s", out)
	}

	// Acked records must be deleted from the processed/ directory.
	spoolDir := emergency.SpoolDir(cityDir)
	for _, id := range ids[:2] {
		ackedPath := filepath.Join(spoolDir, "processed", id+".json")
		if _, err := os.Stat(ackedPath); !os.IsNotExist(err) {
			t.Fatalf("doctor --fix: processed record %s should be pruned but still exists (stat err: %v)", id, err)
		}
	}

	// The still-open record must remain in the spool root.
	openPath := filepath.Join(spoolDir, ids[2]+".json")
	if _, err := os.Stat(openPath); err != nil {
		t.Fatalf("doctor --fix: open record %s must remain but stat failed: %v", ids[2], err)
	}
}
