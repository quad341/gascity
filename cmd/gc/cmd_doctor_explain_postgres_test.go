package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestExplainPostgresBootstrap_PrintsDocBody(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := doDoctorWithOptions(false, false, false, true, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if got, want := stdout.String(), readPostgresBootstrapSource(t); got != want {
		t.Fatalf("stdout differs from engdoc\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestExplainPostgresBootstrap_ExitCodeZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := newDoctorCmd(&stdout, &stderr)
	cmd.SetArgs([]string{"--explain-postgres-bootstrap"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v; stderr=%q", err, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatal("stdout empty, want bootstrap doc")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestExplainPostgresBootstrap_NoOtherChecksRun(t *testing.T) {
	oldCityFlag := cityFlag
	cityFlag = filepath.Join(t.TempDir(), "missing-city")
	t.Cleanup(func() { cityFlag = oldCityFlag })

	var stdout, stderr bytes.Buffer
	code := doDoctorWithOptions(false, false, false, true, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Local PostgreSQL bootstrap for bd PG-backed scopes") {
		t.Fatalf("stdout missing bootstrap doc:\n%s", stdout.String())
	}
}

func TestExplainPostgresBootstrap_ShellBlocksParseCleanly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := doDoctorWithOptions(false, false, false, true, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	for i, block := range extractPostgresBootstrapBashBlocks(stdout.String()) {
		cmd := exec.Command("bash", "-n")
		cmd.Stdin = strings.NewReader(block)
		var parseErr bytes.Buffer
		cmd.Stderr = &parseErr
		if err := cmd.Run(); err != nil {
			t.Fatalf("bash block %d does not parse: %v\n%s", i+1, err, parseErr.String())
		}
	}
}

func TestExplainPostgresBootstrap_NotInteractWithFix(t *testing.T) {
	oldCityFlag := cityFlag
	cityFlag = filepath.Join(t.TempDir(), "missing-city")
	t.Cleanup(func() { cityFlag = oldCityFlag })

	var stdout, stderr bytes.Buffer
	code := doDoctorWithOptions(true, true, true, true, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if got, want := stdout.String(), readPostgresBootstrapSource(t); got != want {
		t.Fatalf("stdout differs from engdoc with --fix path\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestExplainPostgresBootstrap_EmbedMatchesSourceFile(t *testing.T) {
	if got, want := postgresBootstrapDoc, readPostgresBootstrapSource(t); got != want {
		t.Fatalf("embedded doc differs from source: got %d bytes, want %d bytes", len(got), len(want))
	}
}

func readPostgresBootstrapSource(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "engdocs", "postgres-local-bootstrap.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func extractPostgresBootstrapBashBlocks(markdown string) []string {
	re := regexp.MustCompile("(?s)```bash\n(.*?)\n```")
	matches := re.FindAllStringSubmatch(markdown, -1)
	blocks := make([]string, 0, len(matches))
	for _, match := range matches {
		blocks = append(blocks, match[1]+"\n")
	}
	return blocks
}
