package lints

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	macOSBootstrapDocPath          = "../../engdocs/postgres-macos-launchd-bootstrap.md"
	macOSBootstrapDoctorSourcePath = "../../internal/doctor/checks_postgres.go"
)

func readPostgresMacOSBootstrapDoc(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(macOSBootstrapDocPath)
	if err != nil {
		t.Fatalf("read %s: %v", macOSBootstrapDocPath, err)
	}
	return string(body)
}

func readPostgresMacOSBootstrapDoctorSource(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(macOSBootstrapDoctorSourcePath)
	if err != nil {
		t.Fatalf("read %s: %v", macOSBootstrapDoctorSourcePath, err)
	}
	return string(body)
}

func TestPostgresMacOSBootstrapDocExists(t *testing.T) {
	if _, err := os.Stat(macOSBootstrapDocPath); err != nil {
		t.Fatalf("stat %s: %v", macOSBootstrapDocPath, err)
	}
}

func TestPostgresMacOSBootstrapDocFrontmatter(t *testing.T) {
	doc := readPostgresMacOSBootstrapDoc(t)
	want := strings.Join([]string{
		"---",
		"title: \"Local PostgreSQL bootstrap for bd PG-backed scopes (macOS launchd)\"",
		"description: \"One-time setup of a private launchd-managed PostgreSQL instance on 127.0.0.1:5433 for local development of bd PG-backed scopes on macOS.\"",
		"---",
		"",
	}, "\n")
	if !strings.HasPrefix(doc, want) {
		t.Fatalf("frontmatter mismatch\nwant prefix:\n%s\ngot:\n%s", want, firstLines(doc, 5))
	}
}

func TestPostgresMacOSBootstrapDocSections(t *testing.T) {
	doc := readPostgresMacOSBootstrapDoc(t)
	re := regexp.MustCompile(`(?m)^## ([0-9]+)\.`)
	matches := re.FindAllStringSubmatch(doc, -1)
	if len(matches) != 11 {
		t.Fatalf("expected 11 numbered H2 sections, got %d", len(matches))
	}
	for i, match := range matches {
		want := string(rune('1' + i))
		switch i {
		case 9:
			want = "10"
		case 10:
			want = "11"
		}
		if match[1] != want {
			t.Fatalf("section %d out of order: got ## %s., want ## %s.", i+1, match[1], want)
		}
	}
}

func TestPostgresMacOSBootstrapDocShellBlocksParse(t *testing.T) {
	doc := readPostgresMacOSBootstrapDoc(t)
	blocks := bashBlocks(doc)
	if len(blocks) == 0 {
		t.Fatal("expected at least one fenced bash block")
	}
	for i, block := range blocks {
		path := filepath.Join(t.TempDir(), "block.sh")
		if err := os.WriteFile(path, []byte(block), 0o600); err != nil {
			t.Fatalf("write shell block %d: %v", i+1, err)
		}
		cmd := exec.Command("bash", "-n", path)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("bash -n failed for block %d: %v\n%s", i+1, err, output)
		}
	}
}

func TestPostgresMacOSBootstrapDocPlistPathConsistent(t *testing.T) {
	doc := readPostgresMacOSBootstrapDoc(t)
	for _, want := range []string{
		"~/Library/LaunchAgents/com.beads.postgres.plist",
		"$HOME/Library/LaunchAgents/com.beads.postgres.plist",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("missing plist path %q", want)
		}
	}
	if strings.Contains(doc, "LaunchDaemons/com.beads.postgres.plist") {
		t.Fatal("doc should not reference a LaunchDaemon plist")
	}
	src := readPostgresMacOSBootstrapDoctorSource(t)
	if !strings.Contains(src, "beadsPostgresMacOSPlistFile") ||
		!strings.Contains(src, `"Library/LaunchAgents/com.beads.postgres.plist"`) {
		t.Fatal("macOS plist helper path does not match doc LaunchAgent path")
	}
}

func TestPostgresMacOSBootstrapDocFixHintReference(t *testing.T) {
	src := readPostgresMacOSBootstrapDoctorSource(t)
	if !strings.Contains(src, "engdocs/postgres-macos-launchd-bootstrap.md") {
		t.Fatalf("internal/doctor/checks_postgres.go missing %q", "engdocs/postgres-macos-launchd-bootstrap.md")
	}
}

func TestPostgresMacOSBootstrapDocCredentialsHeredocUnquoted(t *testing.T) {
	doc := readPostgresMacOSBootstrapDoc(t)
	if !strings.Contains(doc, "cat > \"$HOME/.config/beads/credentials\" <<EOF") {
		t.Fatal("credentials heredoc must use unquoted <<EOF so PG_PASSWORD expands")
	}
	if strings.Contains(doc, "cat > \"$HOME/.config/beads/credentials\" <<'EOF'") {
		t.Fatal("credentials heredoc must not quote EOF")
	}
}

func TestPostgresMacOSBootstrapDocPort5433Pinned(t *testing.T) {
	doc := readPostgresMacOSBootstrapDoc(t)
	required := []string{
		"127.0.0.1:5433",
		"TCP:5433",
		"port = 5433",
		"-p 5433",
		"[127.0.0.1:5433]",
	}
	for _, want := range required {
		if !strings.Contains(doc, want) {
			t.Fatalf("missing required 5433 reference %q", want)
		}
	}
	if strings.Contains(doc, "127.0.0.1:5432") {
		t.Fatal("doc must not target 127.0.0.1:5432")
	}
	if got := strings.Count(doc, "TCP:5432"); got != 1 {
		t.Fatalf("expected exactly one TCP:5432 warning probe, got %d", got)
	}
}

func TestPostgresMacOSBootstrapDocNoLinuxServiceCommands(t *testing.T) {
	doc := readPostgresMacOSBootstrapDoc(t)
	for _, forbidden := range []string{"ss -tln", "systemctl", "loginctl"} {
		if strings.Contains(doc, forbidden) {
			t.Fatalf("doc contains Linux-specific command %q", forbidden)
		}
	}
}

func TestPostgresMacOSBootstrapDocIdempotencyMessages(t *testing.T) {
	doc := readPostgresMacOSBootstrapDoc(t)
	expected := []string{
		"FATAL: $HOME/Library/LaunchAgents/com.beads.postgres.plist already exists.",
		"       remove it (and re-run §11 uninstallation if needed) before re-bootstrapping.",
		"FATAL: $HOME/.local/share/beads/postgres/data already exists.",
		"       remove it (rm -rf) before re-bootstrapping. this destroys any data already there.",
		"FATAL: $HOME/.config/beads/credentials already exists.",
		"       remove it (and any associated server) before re-bootstrapping.",
		"FATAL: port 5433 is already in use.",
		"       another process is bound to 127.0.0.1:5433. stop it before re-running.",
		"WARNING: a PostgreSQL instance appears active on port 5432.",
		"         this bootstrap installs a SEPARATE private instance on port 5433.",
		"         the two will coexist; you will have two running PostgreSQL servers.",
		"         press Enter to continue, or Ctrl-C to abort.",
		"FATAL: pg_ctl not found on PATH.",
		"       install PostgreSQL >= 14 via Homebrew: brew install postgresql@16",
		"FATAL: pg_ctl version ${PG_VERSION_MAJOR} < 14.",
		"       this bootstrap requires PostgreSQL >= 14 (scram-sha-256 defaults).",
		"FATAL: 'postgres' binary not found on PATH.",
	}
	for _, want := range expected {
		if !strings.Contains(doc, want) {
			t.Fatalf("missing idempotency/prerequisite message %q", want)
		}
	}
}

func bashBlocks(doc string) []string {
	var blocks []string
	const fence = "```bash\n"
	for {
		start := strings.Index(doc, fence)
		if start == -1 {
			return blocks
		}
		doc = doc[start+len(fence):]
		end := strings.Index(doc, "\n```")
		if end == -1 {
			return blocks
		}
		blocks = append(blocks, doc[:end])
		doc = doc[end+len("\n```"):]
	}
}

func firstLines(s string, n int) string {
	lines := strings.SplitAfter(s, "\n")
	if len(lines) < n {
		n = len(lines)
	}
	return strings.Join(lines[:n], "")
}
