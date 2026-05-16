package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/beads/contract"
	"github.com/gastownhall/gascity/internal/config"
)

const (
	systemdRuntimeDir = "/run/systemd/system"
	systemdLingerDir  = "/var/lib/systemd/linger"

	postgresServerLingerAmendment = "also run `loginctl enable-linger` so PG starts after reboot (per-user systemd otherwise stops on logout)"
	postgresServerLingerDetail    = "⚠ systemd-user linger is not enabled — PG will not start at boot"
)

var (
	systemdUserLingerStatProbe   = os.Stat
	systemdUserLingerCurrentUser = user.Current
	systemdUserLingerGOOS        = func() string { return runtime.GOOS }
	postgresServerDialContext    = func(ctx context.Context, network, address string) (net.Conn, error) {
		dialer := net.Dialer{Timeout: 2 * time.Second}
		return dialer.DialContext(ctx, network, address)
	}
)

// PostgresServerCheck verifies that each Postgres-backed bead scope's
// listener accepts a TCP connection. It does not authenticate.
type PostgresServerCheck struct {
	cityPath string
	cfg      *config.City
}

// NewPostgresServerCheck creates a check for city and rig Postgres listeners.
func NewPostgresServerCheck(cityPath string, cfg *config.City) *PostgresServerCheck {
	return &PostgresServerCheck{cityPath: cityPath, cfg: cfg}
}

// Name returns the check identifier.
func (c *PostgresServerCheck) Name() string { return "postgres-server" }

// Run checks whether every Postgres-backed scope accepts TCP connections.
func (c *PostgresServerCheck) Run(ctx *CheckContext) *CheckResult {
	if ctx == nil {
		ctx = &CheckContext{}
	}
	scopes := postgresBackedScopes(c.cityPath, c.cfg)
	findings := make([]postgresServerFinding, 0, len(scopes))
	for _, scope := range scopes {
		findings = append(findings, evaluatePostgresServerScope(scope))
	}
	sortPostgresServerFindings(findings)

	result := &CheckResult{Name: c.Name()}
	if len(findings) == 0 {
		result.Status = StatusOK
		result.Message = "no postgres-backed scopes"
		return result
	}

	lingerDisabled, lingerErr := postgresServerLingerStatus(scopes)
	result.Status, result.Message = postgresServerAggregateMessage(findings, lingerDisabled)
	result.FixHint = postgresServerAggregateFixHint(findings, result.Status, lingerDisabled)
	if ctx.Verbose {
		result.Details = postgresServerDetails(findings, lingerDisabled, lingerErr)
	}
	return result
}

// CanFix returns false because Postgres is externally managed.
func (c *PostgresServerCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *PostgresServerCheck) Fix(_ *CheckContext) error { return nil }

type postgresServerScope struct {
	kind string
	name string
	key  string
	root string
	host string
	port string
}

type postgresServerFinding struct {
	scope   postgresServerScope
	status  CheckStatus
	message string
}

func postgresBackedScopes(cityPath string, cfg *config.City) []postgresServerScope {
	if cfg == nil {
		return nil
	}
	cityPath = filepath.Clean(cityPath)
	var scopes []postgresServerScope
	if meta, ok := loadPostgresServerMetadata(cityPath); ok {
		scopes = append(scopes, newPostgresServerScope("city", "", cityPath, meta))
	}
	for _, rig := range cfg.Rigs {
		if strings.TrimSpace(rig.Path) == "" {
			continue
		}
		root := resolvePostgresAuthScopeRoot(cityPath, rig.Path)
		meta, ok := loadPostgresServerMetadata(root)
		if !ok {
			continue
		}
		name := strings.TrimSpace(rig.Name)
		if name == "" {
			name = filepath.Base(filepath.Clean(root))
		}
		scopes = append(scopes, newPostgresServerScope("rig", name, root, meta))
	}
	sort.Slice(scopes, func(i, j int) bool {
		return scopes[i].key < scopes[j].key
	})
	return scopes
}

func loadPostgresServerMetadata(scopeRoot string) (contract.MetadataState, bool) {
	data, err := os.ReadFile(filepath.Join(scopeRoot, ".beads", "metadata.json"))
	if err != nil {
		return contract.MetadataState{}, false
	}
	var meta contract.MetadataState
	if err := json.Unmarshal(data, &meta); err != nil {
		return contract.MetadataState{}, false
	}
	if strings.TrimSpace(meta.Backend) != "postgres" {
		return contract.MetadataState{}, false
	}
	return meta, true
}

func newPostgresServerScope(kind, name, root string, meta contract.MetadataState) postgresServerScope {
	key := "city"
	if kind == "rig" {
		key = "rigs/" + name
	}
	return postgresServerScope{
		kind: kind,
		name: name,
		key:  key,
		root: filepath.Clean(root),
		host: meta.PostgresHost,
		port: meta.PostgresPort,
	}
}

func evaluatePostgresServerScope(scope postgresServerScope) postgresServerFinding {
	if scope.host == "" || scope.port == "" {
		return postgresServerFinding{
			scope:   scope,
			status:  StatusError,
			message: "metadata missing postgres host/port; cannot probe",
		}
	}
	addr := scope.address()
	conn, err := postgresServerDialContext(context.Background(), "tcp", addr)
	if err != nil {
		return postgresServerFinding{
			scope:   scope,
			status:  StatusError,
			message: "server not reachable at " + addr,
		}
	}
	_ = conn.Close()
	return postgresServerFinding{
		scope:   scope,
		status:  StatusOK,
		message: "reachable at " + addr,
	}
}

func postgresServerAggregateMessage(findings []postgresServerFinding, lingerDisabled bool) (CheckStatus, string) {
	if len(findings) == 1 {
		finding := findings[0]
		if finding.status == StatusError {
			return StatusError, finding.message
		}
		if lingerDisabled {
			return StatusWarning, finding.message + "; boot-survival is not configured"
		}
		return StatusOK, finding.message
	}

	firstError, hasError := firstPostgresServerFindingAtStatus(findings, StatusError)
	if hasError {
		return StatusError, fmt.Sprintf("%d postgres-backed scope(s); first issue: %s", len(findings), firstError.message)
	}
	if lingerDisabled {
		return StatusWarning, "all postgres-backed scopes reachable, but boot-survival is not configured"
	}
	return StatusOK, fmt.Sprintf("%d postgres-backed scope(s) reachable", len(findings))
}

func postgresServerAggregateFixHint(findings []postgresServerFinding, status CheckStatus, lingerDisabled bool) string {
	switch status {
	case StatusWarning:
		return postgresServerFixHint("", "", systemdUserLingerGOOS(), true)
	case StatusError:
		firstError, ok := firstPostgresServerFindingAtStatus(findings, StatusError)
		if !ok || firstError.scope.host == "" || firstError.scope.port == "" {
			if lingerDisabled {
				return postgresServerFixHint("", "", systemdUserLingerGOOS(), true)
			}
			return ""
		}
		return postgresServerFixHint(firstError.scope.host, firstError.scope.port, systemdUserLingerGOOS(), lingerDisabled)
	default:
		return ""
	}
}

func postgresServerDetails(findings []postgresServerFinding, lingerDisabled bool, lingerErr error) []string {
	rows := make([]postgresServerDetailRow, 0, len(findings)+2)
	for _, finding := range findings {
		rows = append(rows, postgresServerDetailRow{
			status: finding.status,
			key:    finding.scope.key,
			text:   finding.detailRow(),
		})
	}
	if lingerDisabled {
		rows = append(rows, postgresServerDetailRow{
			status: StatusWarning,
			global: true,
			text:   postgresServerLingerDetail,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].status != rows[j].status {
			return rows[i].status > rows[j].status
		}
		if rows[i].global != rows[j].global {
			return !rows[i].global
		}
		return rows[i].key < rows[j].key
	})
	details := make([]string, 0, len(rows)+1)
	for _, row := range rows {
		details = append(details, row.text)
	}
	if lingerErr != nil {
		details = append(details, "linger probe failed: "+lingerErr.Error()+"; PG boot-survival not verified")
	}
	return details
}

type postgresServerDetailRow struct {
	status CheckStatus
	key    string
	global bool
	text   string
}

func firstPostgresServerFindingAtStatus(findings []postgresServerFinding, status CheckStatus) (postgresServerFinding, bool) {
	for _, finding := range findings {
		if finding.status == status {
			return finding, true
		}
	}
	return postgresServerFinding{}, false
}

func sortPostgresServerFindings(findings []postgresServerFinding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].status != findings[j].status {
			return findings[i].status > findings[j].status
		}
		return findings[i].scope.key < findings[j].scope.key
	})
}

func (f postgresServerFinding) detailRow() string {
	return postgresServerStatusGlyph(f.status) + " " + f.scope.display() + " — " + f.message
}

func (s postgresServerScope) display() string {
	return s.key + " (" + s.address() + ")"
}

func (s postgresServerScope) address() string {
	return net.JoinHostPort(s.host, s.port)
}

func postgresServerStatusGlyph(status CheckStatus) string {
	switch status {
	case StatusError:
		return "✗"
	case StatusWarning:
		return "⚠"
	default:
		return "✓"
	}
}

func postgresServerLingerStatus(scopes []postgresServerScope) (bool, error) {
	if systemdUserLingerGOOS() != "linux" || !postgresServerHasLoopbackScope(scopes) {
		return false, nil
	}
	systemdPresent, err := postgresServerSystemdRuntimePresent()
	if err != nil || !systemdPresent {
		return false, err
	}
	enabled, err := systemdUserLingerEnabled()
	if err != nil {
		return false, err
	}
	return !enabled, nil
}

func postgresServerSystemdRuntimePresent() (bool, error) {
	if _, err := systemdUserLingerStatProbe(systemdRuntimeDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func postgresServerHasLoopbackScope(scopes []postgresServerScope) bool {
	for _, scope := range scopes {
		if postgresServerLoopbackHost(scope.host) {
			return true
		}
	}
	return false
}

func postgresServerLoopbackHost(host string) bool {
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

// postgresServerFixHint returns the operator-facing FixHint for the
// postgres-server check.
func postgresServerFixHint(host, port, goos string, lingerNeeded bool) string {
	_ = port
	var base string
	switch goos {
	case "linux":
		base = "start PG (e.g. systemctl --user start postgresql, sudo systemctl start postgresql, or docker compose up -d postgres) then re-run gc doctor"
	case "darwin":
		base = "start PG (e.g. brew services start postgresql@<version>, launch Postgres.app, or docker compose up -d postgres) then re-run gc doctor"
	case "windows":
		base = "start the PostgreSQL service (services.msc → PostgreSQL → Start, or pg_ctl start -D <data-dir>) then re-run gc doctor"
	default:
		base = "gc does not manage external PostgreSQL servers; start it via your OS supervisor or container runtime, then re-run gc doctor"
	}
	if host != "" && !postgresServerLoopbackHost(host) {
		base += "; or check the cloud provider's console / your VPN if this is a remote host"
	}
	if !lingerNeeded {
		return base
	}
	if host == "" {
		return postgresServerLingerAmendment
	}
	return postgresServerLingerAmendment + " ; " + base
}

// systemdUserLingerEnabled reports whether the current user has systemd-user
// linger enabled. It performs only file probes and never executes commands.
func systemdUserLingerEnabled() (bool, error) {
	if systemdUserLingerGOOS() != "linux" {
		return false, nil
	}
	if _, err := systemdUserLingerStatProbe(systemdRuntimeDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	current, err := systemdUserLingerCurrentUser()
	if err != nil {
		return false, err
	}
	_, err = systemdUserLingerStatProbe(filepath.Join(systemdLingerDir, current.Username))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
