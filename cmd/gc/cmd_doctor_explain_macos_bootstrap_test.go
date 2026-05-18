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

func TestExplainPostgresMacOSBootstrap_PrintsDocBody(t *testing.T) {
	stdout, stderr, err := executeDoctorPostgresMacOSExplain(t)
	if err != nil {
		t.Fatalf("doctor explain returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if want := readPostgresMacOSBootstrapSource(t); stdout != want {
		t.Fatalf("stdout does not match engdocs/postgres-macos-launchd-bootstrap.md")
	}
}

func TestExplainPostgresMacOSBootstrap_ExitCodeZero(t *testing.T) {
	_, stderr, err := executeDoctorPostgresMacOSExplain(t)
	if err != nil {
		t.Fatalf("doctor explain returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestExplainPostgresMacOSBootstrap_NoOtherChecksRun(t *testing.T) {
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

	_, stderr, err := executeDoctorPostgresMacOSExplain(t)
	if err != nil {
		t.Fatalf("doctor explain returned error: %v\nstderr:\n%s", err, stderr)
	}
}

func TestExplainPostgresMacOSBootstrap_ShellBlocksParseCleanly(t *testing.T) {
	stdout, stderr, err := executeDoctorPostgresMacOSExplain(t)
	if err != nil {
		t.Fatalf("doctor explain returned error: %v\nstderr:\n%s", err, stderr)
	}

	blocks := postgresMacOSExplainFencedBlocks(stdout, "bash")
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

func TestExplainPostgresMacOSBootstrap_EmbedMatchesSourceFile(t *testing.T) {
	if postgresMacOSBootstrapDoc != readPostgresMacOSBootstrapSource(t) {
		t.Fatal("embedded postgres macOS bootstrap doc does not match source file")
	}
}

func executeDoctorPostgresMacOSExplain(t *testing.T) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := newDoctorCmd(&stdout, &stderr)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--explain-postgres-macos-launchd-bootstrap"})
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func readPostgresMacOSBootstrapSource(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	body, err := os.ReadFile(filepath.Join(repoRoot, "engdocs", "postgres-macos-launchd-bootstrap.md"))
	if err != nil {
		t.Fatalf("read postgres macOS bootstrap source: %v", err)
	}
	return string(body)
}

var postgresMacOSExplainFenceRE = regexp.MustCompile("(?ms)^```([A-Za-z0-9_-]*)[^\\n]*\\n(.*?)^```")

func postgresMacOSExplainFencedBlocks(doc, lang string) []string {
	matches := postgresMacOSExplainFenceRE.FindAllStringSubmatch(doc, -1)
	var blocks []string
	for _, match := range matches {
		if match[1] == lang {
			blocks = append(blocks, match[2])
		}
	}
	return blocks
}
