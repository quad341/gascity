package lints

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const postgresNonSystemdDoc = "engdocs/postgres-non-systemd-linux-bootstrap.md"

func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..")
}

func readPostgresNonSystemdDoc(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), postgresNonSystemdDoc))
	if err != nil {
		t.Fatalf("read %s: %v", postgresNonSystemdDoc, err)
	}
	return string(data)
}

func TestPostgresNonSystemdBootstrapDocExists(t *testing.T) {
	path := filepath.Join(repoRoot(t), postgresNonSystemdDoc)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat %s: %v", postgresNonSystemdDoc, err)
	}
}

func TestPostgresNonSystemdBootstrapDocFrontmatter(t *testing.T) {
	doc := readPostgresNonSystemdDoc(t)
	want := `---
title: "Local PostgreSQL bootstrap for bd PG-backed scopes (non-systemd Linux: OpenRC, runit, s6)"
description: "One-time setup of a private PostgreSQL instance on 127.0.0.1:5433 for local development of bd PG-backed scopes on Linux systems using OpenRC, runit, or s6 instead of systemd."
---`
	if !strings.HasPrefix(doc, want+"\n\n") {
		t.Fatalf("frontmatter mismatch\nwant prefix:\n%s\n\ngot prefix:\n%s", want, firstN(doc, len(want)+80))
	}
}

func TestPostgresNonSystemdBootstrapDocSections(t *testing.T) {
	doc := readPostgresNonSystemdDoc(t)
	last := -1
	for i := 1; i <= 11; i++ {
		heading := "## " + strconv.Itoa(i) + "."
		idx := strings.Index(doc, heading)
		if idx < 0 {
			t.Fatalf("missing heading %q", heading)
		}
		if idx <= last {
			t.Fatalf("heading %q is out of order", heading)
		}
		last = idx
	}
	section6 := sectionBetween(doc, "## 6.", "## 7.")
	for _, heading := range []string{"### 6.1 OpenRC", "### 6.2 runit", "### 6.3 s6"} {
		if !strings.Contains(section6, heading) {
			t.Fatalf("section 6 missing %q", heading)
		}
	}
}

func TestPostgresNonSystemdBootstrapDocShellBlocksParse(t *testing.T) {
	doc := readPostgresNonSystemdDoc(t)
	blocks := fencedBlocks(doc, "bash")
	if len(blocks) == 0 {
		t.Fatal("no bash fenced blocks found")
	}
	for i, block := range blocks {
		cmd := exec.Command("bash", "-n")
		cmd.Stdin = strings.NewReader(block)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("bash block %d does not parse: %v\n%s", i+1, err, stderr.String())
		}
	}
}

func TestPostgresNonSystemdBootstrapDocPort5433Pinned(t *testing.T) {
	doc := readPostgresNonSystemdDoc(t)
	if !strings.Contains(doc, "127.0.0.1:5433") {
		t.Fatal("missing pinned loopback port 127.0.0.1:5433")
	}
	section2 := sectionBetween(doc, "## 2.", "## 3.")
	if got, want := strings.Count(doc, "5432"), strings.Count(section2, "5432"); got != want {
		t.Fatal("5432 references must stay confined to §2 warning")
	}
}

func TestPostgresNonSystemdBootstrapDocCredentialsHeredocUnquoted(t *testing.T) {
	section7 := sectionBetween(readPostgresNonSystemdDoc(t), "## 7.", "## 8.")
	if !strings.Contains(section7, `cat > "$HOME/.config/beads/credentials" <<EOF`) {
		t.Fatal("credentials heredoc must use unquoted <<EOF so PG_PASSWORD expands")
	}
}

func TestPostgresNonSystemdBootstrapDocNoSystemdCommands(t *testing.T) {
	doc := readPostgresNonSystemdDoc(t)
	for _, forbidden := range []string{"systemctl", "loginctl"} {
		if strings.Contains(doc, forbidden) {
			t.Fatalf("doc must not contain %q", forbidden)
		}
	}
}

func TestPostgresNonSystemdBootstrapDocInitSystemSections(t *testing.T) {
	doc := readPostgresNonSystemdDoc(t)
	assertSectionContains(t, doc, "### 6.1 OpenRC", "### 6.2 runit", "/etc/init.d/beads-postgres")
	assertSectionContains(t, doc, "### 6.2 runit", "### 6.3 s6", "~/.local/sv/beads-postgres")
	assertSectionContains(t, doc, "### 6.3 s6", "## 7.", `S6_USER_SERVICES_DIR="${HOME}/.s6/service"`)
	assertSectionContains(t, doc, "### 6.3 s6", "## 7.", `${S6_USER_SERVICES_DIR}/beads-postgres`)
}

func assertSectionContains(t *testing.T, doc, start, end, want string) {
	t.Helper()
	section := sectionBetween(doc, start, end)
	if !strings.Contains(section, want) {
		t.Fatalf("section %q missing %q", start, want)
	}
}

func sectionBetween(doc, start, end string) string {
	startIdx := strings.Index(doc, start)
	if startIdx < 0 {
		return ""
	}
	endIdx := strings.Index(doc[startIdx:], end)
	if endIdx < 0 {
		return doc[startIdx:]
	}
	return doc[startIdx : startIdx+endIdx]
}

var fenceRE = regexp.MustCompile("(?ms)^```([A-Za-z0-9_-]*)[^\n]*\n(.*?)^```")

func fencedBlocks(doc, lang string) []string {
	matches := fenceRE.FindAllStringSubmatch(doc, -1)
	var blocks []string
	for _, match := range matches {
		if match[1] == lang {
			blocks = append(blocks, match[2])
		}
	}
	return blocks
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
