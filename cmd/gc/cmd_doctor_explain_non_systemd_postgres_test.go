package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/engdocs"
)

func TestExplainPostgresNonSystemdBootstrap_PrintsDocBody(t *testing.T) {
	code, stdout, stderr := runExplainPostgresNonSystemdBootstrap(t)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr)
	}

	want := readPostgresNonSystemdBootstrapSource(t)
	if stdout != want {
		t.Fatalf("stdout does not match source doc\nwant %d bytes, got %d bytes", len(want), len(stdout))
	}
}

func TestExplainPostgresNonSystemdBootstrap_ExitCodeZero(t *testing.T) {
	code, _, stderr := runExplainPostgresNonSystemdBootstrap(t)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestExplainPostgresNonSystemdBootstrap_NoOtherChecksRun(t *testing.T) {
	oldRunDoctor := runDoctor
	called := false
	runDoctor = func(bool, bool, bool, io.Writer, io.Writer) int {
		called = true
		return 1
	}
	t.Cleanup(func() { runDoctor = oldRunDoctor })

	code, _, stderr := runExplainPostgresNonSystemdBootstrap(t)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr)
	}
	if called {
		t.Fatal("doctor runner was called; explain mode must short-circuit before checks")
	}
}

func TestExplainPostgresNonSystemdBootstrap_ShellBlocksParseCleanly(t *testing.T) {
	code, stdout, stderr := runExplainPostgresNonSystemdBootstrap(t)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr)
	}

	blocks := fencedPostgresNonSystemdBootstrapBlocks(stdout, "bash")
	if len(blocks) == 0 {
		t.Fatal("no bash fenced blocks found")
	}
	for i, block := range blocks {
		cmd := exec.Command("bash", "-n")
		cmd.Stdin = strings.NewReader(block)
		var parseStderr bytes.Buffer
		cmd.Stderr = &parseStderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("bash block %d does not parse: %v\n%s", i+1, err, parseStderr.String())
		}
	}
}

func TestExplainPostgresNonSystemdBootstrap_EmbedMatchesSourceFile(t *testing.T) {
	want := readPostgresNonSystemdBootstrapSource(t)
	got := engdocs.PostgresNonSystemdLinuxBootstrap()
	if got != want {
		t.Fatalf("embedded doc does not match source file\nwant %d bytes, got %d bytes", len(want), len(got))
	}
}

func runExplainPostgresNonSystemdBootstrap(t *testing.T) (int, string, string) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	cmd := newDoctorCmd(&stdout, &stderr)
	cmd.SetArgs([]string{"--explain-postgres-non-systemd-linux-bootstrap"})
	if err := cmd.Execute(); err != nil {
		if errors.Is(err, errExit) {
			return 1, stdout.String(), stderr.String()
		}
		return 2, stdout.String(), stderr.String() + err.Error()
	}
	return 0, stdout.String(), stderr.String()
}

func readPostgresNonSystemdBootstrapSource(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(filename), "..", "..", "engdocs", "postgres-non-systemd-linux-bootstrap.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

var postgresNonSystemdBootstrapFenceRE = regexp.MustCompile("(?ms)^```([A-Za-z0-9_-]*)[^\n]*\n(.*?)^```")

func fencedPostgresNonSystemdBootstrapBlocks(doc, lang string) []string {
	matches := postgresNonSystemdBootstrapFenceRE.FindAllStringSubmatch(doc, -1)
	var blocks []string
	for _, match := range matches {
		if match[1] == lang {
			blocks = append(blocks, match[2])
		}
	}
	return blocks
}
