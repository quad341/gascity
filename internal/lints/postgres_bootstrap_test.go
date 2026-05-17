package lints

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const postgresBootstrapDocPath = "engdocs/postgres-local-bootstrap.md"

func TestPostgresBootstrapDocExists(t *testing.T) {
	if _, err := os.Stat(repoPath(postgresBootstrapDocPath)); err != nil {
		t.Fatalf("%s missing: %v", postgresBootstrapDocPath, err)
	}
}

func TestPostgresBootstrapDocFrontmatter(t *testing.T) {
	lines := strings.Split(readPostgresBootstrapDoc(t), "\n")
	want := []string{
		"---",
		`title: "Local PostgreSQL bootstrap for bd PG-backed scopes"`,
		`description: "One-time setup of a private systemd-user PostgreSQL instance on 127.0.0.1:5433 for local development of bd PG-backed scopes."`,
		"---",
	}
	if len(lines) < len(want) {
		t.Fatalf("doc has %d lines, want at least %d", len(lines), len(want))
	}
	for i, wantLine := range want {
		if lines[i] != wantLine {
			t.Fatalf("frontmatter line %d = %q, want %q", i+1, lines[i], wantLine)
		}
	}
}

func TestPostgresBootstrapDocSections(t *testing.T) {
	doc := readPostgresBootstrapDoc(t)
	sections := []string{
		"## 1. Audience and prerequisites",
		"## 2. Detect existing state",
		"## 3. Initialize the data directory",
		"## 4. Configure the listener",
		"## 5. Set the role password",
		"## 6. Install the systemd-user unit",
		"## 7. Populate the credentials file",
		"## 8. Configure the user-services environment",
		"## 9. Enable linger (mandatory for boot survival)",
		"## 10. Verify",
		"## 11. Uninstallation",
	}
	last := -1
	for _, section := range sections {
		idx := strings.Index(doc, section)
		if idx < 0 {
			t.Fatalf("doc missing section %q", section)
		}
		if idx <= last {
			t.Fatalf("section %q appears out of order", section)
		}
		last = idx
	}
}

func TestPostgresBootstrapDocShellBlocksParse(t *testing.T) {
	for i, block := range extractBashBlocks(readPostgresBootstrapDoc(t)) {
		cmd := exec.Command("bash", "-n")
		cmd.Stdin = strings.NewReader(block)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("bash block %d does not parse: %v\n%s", i+1, err, stderr.String())
		}
	}
}

func TestPostgresBootstrapDocFixHintReference(t *testing.T) {
	src, err := os.ReadFile(repoPath("internal/doctor/checks_postgres.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), postgresBootstrapDocPath) {
		t.Fatalf("internal/doctor/checks_postgres.go missing %q", postgresBootstrapDocPath)
	}
}

func TestPostgresBootstrapDocUnitFilePathConsistent(t *testing.T) {
	doc := readPostgresBootstrapDoc(t)
	if !strings.Contains(doc, "~/.config/systemd/user/beads-postgres.service") {
		t.Fatal("doc missing ~/.config/systemd/user/beads-postgres.service")
	}
	src, err := os.ReadFile(repoPath("internal/doctor/checks_postgres.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `.config/systemd/user/beads-postgres.service`) {
		t.Fatal("beadsPostgresUnitInstalled helper path does not match doc unit path")
	}
}

func TestPostgresBootstrapDocCredentialsHeredocUnquoted(t *testing.T) {
	doc := readPostgresBootstrapDoc(t)
	if !strings.Contains(doc, `cat > "$HOME/.config/beads/credentials" <<EOF`) {
		t.Fatal("credentials heredoc must be unquoted so PG_PASSWORD expands")
	}
	if strings.Contains(doc, `cat > "$HOME/.config/beads/credentials" <<'EOF'`) {
		t.Fatal("credentials heredoc is quoted and would write literal $PG_PASSWORD")
	}
}

func TestPostgresBootstrapDocPort5433Pinned(t *testing.T) {
	doc := readPostgresBootstrapDoc(t)
	for _, want := range []string{
		"127.0.0.1:5433",
		"port = 5433",
		"-p 5433",
		"[127.0.0.1:5433]",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("doc missing required port pin %q", want)
		}
	}
	for _, forbidden := range []string{
		"port = 5432",
		"-p 5432",
		"[127.0.0.1:5432]",
		"127.0.0.1:5432",
	} {
		if strings.Contains(doc, forbidden) {
			t.Fatalf("doc contains forbidden postgres target port reference %q", forbidden)
		}
	}
}

func repoPath(path string) string {
	return filepath.Join("..", "..", filepath.FromSlash(path))
}

func readPostgresBootstrapDoc(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(repoPath(postgresBootstrapDocPath))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func extractBashBlocks(markdown string) []string {
	re := regexp.MustCompile("(?s)```bash\n(.*?)\n```")
	matches := re.FindAllStringSubmatch(markdown, -1)
	blocks := make([]string, 0, len(matches))
	for _, match := range matches {
		blocks = append(blocks, match[1]+"\n")
	}
	return blocks
}
