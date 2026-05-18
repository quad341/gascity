package lints

import (
	"bytes"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const postgresContainerBootstrapDoc = "../../engdocs/postgres-container-bootstrap.md"

func readPostgresContainerBootstrapDoc(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(postgresContainerBootstrapDoc)
	if err != nil {
		t.Fatalf("read %s: %v", postgresContainerBootstrapDoc, err)
	}
	return string(body)
}

func TestPostgresContainerBootstrapDocExists(t *testing.T) {
	if _, err := os.Stat(postgresContainerBootstrapDoc); err != nil {
		t.Fatalf("stat %s: %v", postgresContainerBootstrapDoc, err)
	}
}

func TestPostgresContainerBootstrapDocFrontmatter(t *testing.T) {
	doc := readPostgresContainerBootstrapDoc(t)
	want := strings.Join([]string{
		"---",
		`title: "Local PostgreSQL bootstrap for bd PG-backed scopes (docker-compose / podman-compose)"`,
		`description: "Container-runtime bootstrap of a private PostgreSQL 16 instance on 127.0.0.1:5433 for local development of bd PG-backed scopes."`,
		"---",
		"",
	}, "\n")
	if !strings.HasPrefix(doc, want) {
		t.Fatalf("frontmatter mismatch\nwant prefix:\n%s\ngot:\n%s", want, firstContainerDocLines(doc, 5))
	}
}

func TestPostgresContainerBootstrapDocSections(t *testing.T) {
	doc := readPostgresContainerBootstrapDoc(t)
	last := -1
	for i := 1; i <= 8; i++ {
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
}

func TestPostgresContainerBootstrapDocShellBlocksParse(t *testing.T) {
	blocks := postgresContainerDocFencedBlocks(readPostgresContainerBootstrapDoc(t), "bash")
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

func TestPostgresContainerBootstrapDocOperatorContract(t *testing.T) {
	doc := readPostgresContainerBootstrapDoc(t)
	for _, want := range []string{
		"image: postgres:16",
		`cat > "$HOME/.config/beads/docker-compose.yml" <<EOF`,
		`cat > "$HOME/.config/beads/.env" <<EOF`,
		`chmod 600 "$HOME/.config/beads/.env"`,
		`"${HOME}/.local/share/beads/postgres/data:/var/lib/postgresql/data"`,
		`"127.0.0.1:5433:5432"`,
		`cat > "$HOME/.config/beads/credentials" <<EOF`,
		`chmod 600 "$HOME/.config/beads/credentials"`,
		"restart: unless-stopped",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("missing container operator contract fragment %q", want)
		}
	}
}

func TestPostgresContainerBootstrapDocFixHintReference(t *testing.T) {
	body, err := os.ReadFile("../../internal/doctor/checks_postgres.go")
	if err != nil {
		t.Fatalf("read postgres doctor check: %v", err)
	}
	source := string(body)
	for _, want := range []string{
		"beadsPostgresContainerRunning",
		"engdocs/postgres-container-bootstrap.md",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("postgres doctor check missing container bootstrap reference %q", want)
		}
	}
}

func TestPostgresContainerBootstrapDocIdempotencyMessages(t *testing.T) {
	doc := readPostgresContainerBootstrapDoc(t)
	expected := []string{
		"FATAL: a container named 'beads-postgres' already exists.",
		"       stop and remove it (docker rm -f beads-postgres) before re-bootstrapping.",
		"FATAL: $HOME/.local/share/beads/postgres/data already exists.",
		"       remove it (rm -rf) before re-bootstrapping. this destroys any data already there.",
		"FATAL: $HOME/.config/beads/credentials already exists.",
		"       remove it (and any associated container) before re-bootstrapping.",
		"FATAL: $HOME/.config/beads/docker-compose.yml already exists.",
		"       remove it before re-bootstrapping.",
		"FATAL: port 5433 is already in use.",
		"       another process is bound to 127.0.0.1:5433. stop it before re-running.",
		"WARNING: a PostgreSQL instance appears active on port 5432.",
		"         this bootstrap installs a SEPARATE container on port 5433.",
		"         the two will coexist.",
		"         press Enter to continue, or Ctrl-C to abort.",
		"FATAL: neither 'docker compose' nor 'podman-compose' found on PATH.",
		"       install Docker Desktop or 'pip install podman-compose' before re-running.",
		"FATAL: beads-postgres did not become ready within 30 seconds.",
		"       check logs: docker logs beads-postgres",
	}
	for _, want := range expected {
		if !strings.Contains(doc, want) {
			t.Fatalf("missing idempotency/prerequisite message %q", want)
		}
	}
}

func TestPostgresContainerBootstrapDocPortReferencesAreIntentional(t *testing.T) {
	doc := readPostgresContainerBootstrapDoc(t)
	if !strings.Contains(doc, "127.0.0.1:5433") {
		t.Fatal("missing pinned host endpoint 127.0.0.1:5433")
	}
	if !strings.Contains(doc, `"127.0.0.1:5433:5432"`) {
		t.Fatal("missing pinned host-to-container port mapping")
	}
	portRE := regexp.MustCompile(`\b[0-9]{4,5}\b`)
	for _, port := range portRE.FindAllString(doc, -1) {
		if port != "5432" && port != "5433" {
			t.Fatalf("unexpected port-like reference %q", port)
		}
	}
}

func TestPostgresContainerBootstrapDocEnvFileMode(t *testing.T) {
	doc := readPostgresContainerBootstrapDoc(t)
	if !strings.Contains(doc, `chmod 600 "$HOME/.config/beads/.env"`) {
		t.Fatal("doc must chmod the .env file to mode 600")
	}
	if !strings.Contains(doc, "`.env` file is the sole persistent store of the password for this bootstrap (mode\n`600`)") {
		t.Fatal("doc must describe .env as mode 600")
	}
}

func TestPostgresContainerBootstrapDocRestartPolicy(t *testing.T) {
	doc := readPostgresContainerBootstrapDoc(t)
	if !strings.Contains(doc, "restart: unless-stopped") {
		t.Fatal("compose YAML must use restart: unless-stopped")
	}
	if strings.Contains(doc, "restart: always") {
		t.Fatal("compose YAML must not use restart: always")
	}
}

func TestPostgresContainerBootstrapDocImage(t *testing.T) {
	doc := readPostgresContainerBootstrapDoc(t)
	if !strings.Contains(doc, "image: postgres:16") {
		t.Fatal("compose YAML must pin postgres:16")
	}
	if strings.Contains(doc, "postgres:latest") {
		t.Fatal("doc must not reference postgres:latest")
	}
}

func TestPostgresContainerBootstrapDocCredentialsHeredocUnquoted(t *testing.T) {
	section4 := postgresContainerDocSection(readPostgresContainerBootstrapDoc(t), "## 4.", "## 5.")
	if !strings.Contains(section4, `cat > "$HOME/.config/beads/credentials" <<EOF`) {
		t.Fatal("credentials heredoc must use unquoted <<EOF so PG_PASSWORD expands")
	}
	if strings.Contains(section4, `cat > "$HOME/.config/beads/credentials" <<'EOF'`) {
		t.Fatal("credentials heredoc must not quote EOF")
	}
}

var postgresContainerDocFenceRE = regexp.MustCompile("(?ms)^```([A-Za-z0-9_-]*)[^\n]*\n(.*?)^```")

func postgresContainerDocFencedBlocks(doc, lang string) []string {
	matches := postgresContainerDocFenceRE.FindAllStringSubmatch(doc, -1)
	var blocks []string
	for _, match := range matches {
		if match[1] == lang {
			blocks = append(blocks, match[2])
		}
	}
	return blocks
}

func postgresContainerDocSection(doc, start, end string) string {
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

func firstContainerDocLines(s string, n int) string {
	lines := strings.SplitAfter(s, "\n")
	if len(lines) < n {
		n = len(lines)
	}
	return strings.Join(lines[:n], "")
}
