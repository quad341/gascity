package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/doctor"
)

func TestExplainPostgresContainerBootstrap_PrintsDocBody(t *testing.T) {
	stdout, stderr, err := executeDoctorPostgresContainerExplain(t)
	if err != nil {
		t.Fatalf("doctor explain returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if want := readPostgresContainerBootstrapSource(t); stdout != want {
		t.Fatalf("stdout does not match engdocs/postgres-container-bootstrap.md")
	}
}

func TestExplainPostgresContainerBootstrap_ExitCodeZero(t *testing.T) {
	_, stderr, err := executeDoctorPostgresContainerExplain(t)
	if err != nil {
		t.Fatalf("doctor explain returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestExplainPostgresContainerBootstrap_NoOtherChecksRun(t *testing.T) {
	cityDir := t.TempDir()
	writeMinimalCityToml(t, cityDir)

	oldCityFlag := cityFlag
	cityFlag = cityDir
	t.Cleanup(func() { cityFlag = oldCityFlag })

	oldCityCheck := newDoctorDoltServerCheck
	newDoctorDoltServerCheck = func(string, bool) *doctor.DoltServerCheck {
		t.Fatal("doctor checks ran during explain mode")
		return nil
	}
	t.Cleanup(func() { newDoctorDoltServerCheck = oldCityCheck })

	_, stderr, err := executeDoctorPostgresContainerExplain(t)
	if err != nil {
		t.Fatalf("doctor explain returned error: %v\nstderr:\n%s", err, stderr)
	}
}

func TestExplainPostgresContainerBootstrap_ShellBlocksParseCleanly(t *testing.T) {
	stdout, stderr, err := executeDoctorPostgresContainerExplain(t)
	if err != nil {
		t.Fatalf("doctor explain returned error: %v\nstderr:\n%s", err, stderr)
	}

	blocks := postgresContainerExplainFencedBlocks(stdout, "bash")
	if len(blocks) == 0 {
		t.Fatal("no bash fenced blocks found")
	}
	for i, block := range blocks {
		cmd := exec.Command("bash", "-n")
		cmd.Stdin = strings.NewReader(block)
		var parseErr bytes.Buffer
		cmd.Stderr = &parseErr
		if err := cmd.Run(); err != nil {
			t.Fatalf("bash block %d does not parse: %v\n%s", i+1, err, parseErr.String())
		}
	}
}

func TestExplainPostgresContainerBootstrap_EmbedMatchesSourceFile(t *testing.T) {
	if postgresContainerBootstrapDoc != readPostgresContainerBootstrapSource(t) {
		t.Fatal("embedded postgres container bootstrap doc does not match source file")
	}
}

func executeDoctorPostgresContainerExplain(t *testing.T) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := newDoctorCmd(&stdout, &stderr)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--explain-postgres-container-bootstrap"})
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func readPostgresContainerBootstrapSource(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	body, err := os.ReadFile(filepath.Join(repoRoot, "engdocs", "postgres-container-bootstrap.md"))
	if err != nil {
		t.Fatalf("read postgres container bootstrap source: %v", err)
	}
	return string(body)
}

var postgresContainerExplainFenceRE = regexp.MustCompile("(?ms)^```([A-Za-z0-9_-]*)[^\\n]*\\n(.*?)^```")

func postgresContainerExplainFencedBlocks(doc, lang string) []string {
	matches := postgresContainerExplainFenceRE.FindAllStringSubmatch(doc, -1)
	var blocks []string
	for _, match := range matches {
		if match[1] == lang {
			blocks = append(blocks, match[2])
		}
	}
	return blocks
}
