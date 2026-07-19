package warmup_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/doctor"
	doctorchecks "github.com/gastownhall/gascity/internal/doctor/checks"
	"github.com/gastownhall/gascity/internal/mail"
	"github.com/gastownhall/gascity/internal/warmup"
)

type staticWarmupCheck struct {
	name    string
	status  doctor.CheckStatus
	message string
}

func (c staticWarmupCheck) Name() string { return c.name }

func (c staticWarmupCheck) Run(_ *doctor.CheckContext) *doctor.CheckResult {
	return &doctor.CheckResult{Name: c.name, Status: c.status, Message: c.message}
}

func (c staticWarmupCheck) CanFix() bool                     { return false }
func (c staticWarmupCheck) Fix(_ *doctor.CheckContext) error { return nil }
func (c staticWarmupCheck) WarmupEligible() bool             { return true }

func scrubWarmupPostgresEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"GC_POSTGRES_PASSWORD", "BEADS_POSTGRES_PASSWORD", "BEADS_CREDENTIALS_FILE"} {
		t.Setenv(key, "")
	}
	t.Setenv("HOME", t.TempDir())
}

func writeWarmupPGMetadata(t *testing.T, scopeRoot string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := `{"backend":"postgres","postgres_host":"db.example.test","postgres_port":"5432","postgres_user":"bd","postgres_database":"beads"}`
	if err := os.WriteFile(filepath.Join(scopeRoot, ".beads", "metadata.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeWarmupPGScopeEnv(t *testing.T, scopeRoot string, contents string, mode os.FileMode) {
	t.Helper()
	envFile := filepath.Join(scopeRoot, ".beads", ".env")
	if err := os.WriteFile(envFile, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(envFile, mode); err != nil {
		t.Fatal(err)
	}
}

func warmupPostgresRig(t *testing.T) (string, *config.City, string) {
	t.Helper()
	cityPath := t.TempDir()
	rigPath := filepath.Join(cityPath, "rigs", "pwu")
	writeWarmupPGMetadata(t, rigPath)
	cfg := &config.City{Rigs: []config.Rig{{Name: "pwu", Path: "rigs/pwu"}}}
	return cityPath, cfg, rigPath
}

func runPostgresWarmup(t *testing.T, cityPath string, cfg *config.City, checks []doctor.Check) (*warmup.WarmupReport, mail.Message) {
	t.Helper()
	mailer := mail.NewFake()
	report, err := warmup.RunWarmupChecks(context.Background(), cityPath, cfg, warmup.WarmupOpts{
		Checks: checks,
		Mailer: mailer,
	})
	if err != nil {
		t.Fatalf("RunWarmupChecks returned error: %v", err)
	}
	if report == nil {
		t.Fatal("RunWarmupChecks returned nil report")
	}
	messages, err := mailer.Inbox("mayor")
	if err != nil {
		t.Fatalf("Inbox returned error: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("sent mail count = %d; want 1", len(messages))
	}
	return report, messages[0]
}

func TestWarmupRunner_PostgresAuthSoleFailure_UsesCustomBody(t *testing.T) {
	t.Run("SoleFailure", func(t *testing.T) {
		scrubWarmupPostgresEnv(t)
		cityPath, cfg, _ := warmupPostgresRig(t)
		check := doctorchecks.NewPostgresAuthCheck(cityPath, cfg)

		report, msg := runPostgresWarmup(t, cityPath, cfg, []doctor.Check{check})
		_, wantBody := check.SoleFailureMail(*report)

		if msg.Subject != doctorchecks.WarmupMailSubject {
			t.Fatalf("subject = %q; want %q", msg.Subject, doctorchecks.WarmupMailSubject)
		}
		if msg.Body != wantBody {
			t.Fatalf("body = %q; want real PostgresAuthCheck SoleFailureMail body %q", msg.Body, wantBody)
		}
		if strings.Contains(msg.Body, "city warm-up:") {
			t.Fatalf("body = %q; want custom postgres-auth body, not generic warmup body", msg.Body)
		}
	})

	t.Run("MixedFailures", func(t *testing.T) {
		scrubWarmupPostgresEnv(t)
		cityPath, cfg, _ := warmupPostgresRig(t)
		check := doctorchecks.NewPostgresAuthCheck(cityPath, cfg)

		_, msg := runPostgresWarmup(t, cityPath, cfg, []doctor.Check{
			check,
			staticWarmupCheck{name: "other-check", status: doctor.StatusError, message: "other failure"},
		})

		if got, want := msg.Subject, "city warm-up: 2 doctor check(s) failed"; got != want {
			t.Fatalf("subject = %q; want generic fallback %q", got, want)
		}
		if !strings.Contains(msg.Body, "city warm-up: 2 doctor check(s) failed") {
			t.Fatalf("body = %q; want generic fallback summary", msg.Body)
		}
		if strings.Contains(msg.Body, "PG-backed scope(s) failed credential resolution") {
			t.Fatalf("body = %q; want generic fallback, not postgres-auth custom body", msg.Body)
		}
	})
}

func TestWarmupMailBodyExcludesSecrets(t *testing.T) {
	scrubWarmupPostgresEnv(t)
	const secret = "super-secret-pg-password"
	cityPath, cfg, rigPath := warmupPostgresRig(t)
	writeWarmupPGScopeEnv(t, rigPath, "BEADS_POSTGRES_PASSWORD="+secret+"\n", 0o644)
	check := doctorchecks.NewPostgresAuthCheck(cityPath, cfg)

	_, msg := runPostgresWarmup(t, cityPath, cfg, []doctor.Check{check})

	if strings.Contains(msg.Body, secret) {
		t.Fatalf("custom warmup mail body leaked postgres password %q:\n%s", secret, msg.Body)
	}
	if !strings.Contains(msg.Body, "credentials file mode") {
		t.Fatalf("body = %q; want credential-resolution failure context without the secret", msg.Body)
	}
}
