package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/beads/contract"
	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/spf13/cobra"
)

type beadsPreflightOptions struct {
	Scope   string
	JSON    bool
	Verbose bool
}

type beadsPreflightCheckFunc func(scope string) (contract.PreflightResult, error)

func newBeadsPreflightCmd(stdout, stderr io.Writer) *cobra.Command {
	var opts beadsPreflightOptions
	cmd := &cobra.Command{
		Use:   "preflight",
		Short: "Check whether a beads scope is safe for native storage",
		Long: `Run a read-only beads backend preflight for native-store activation.

The command checks provider contract, metadata backend, bd context agreement,
database identity, and metadata field shape. It never rewrites metadata, never
runs repair commands, and has no --fix flag; repair is operator-triggered
outside this command.

Exit codes: ELIGIBLE=0, BLOCKED=1, DEGRADED=2, unable-to-run=3.`,
		Example: `  gc beads preflight
  gc beads preflight --scope /path/to/city --json
  gc beads preflight --verbose`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return exitForCode(cmdBeadsPreflight(opts, stdout, stderr))
		},
	}
	cmd.Flags().StringVar(&opts.Scope, "scope", "", "beads scope root (default: resolved city root)")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "emit redacted machine-readable JSON")
	cmd.Flags().BoolVar(&opts.Verbose, "verbose", false, "include redacted diagnostic detail fields in human output")
	return cmd
}

func cmdBeadsPreflight(opts beadsPreflightOptions, stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc beads preflight: %v\n", err) //nolint:errcheck // best-effort stderr
		return 3
	}
	scopeRoot := resolvePreflightScope(cityPath, opts.Scope)
	opts.Scope = scopeRoot
	checker := newBeadsPreflightChecker(cityPath, scopeRoot)
	return doBeadsPreflight(opts, checker.Check, stdout, stderr)
}

func resolvePreflightScope(cityPath, scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return filepath.Clean(cityPath)
	}
	return filepath.Clean(resolveStoreScopeRoot(cityPath, scope))
}

func newBeadsPreflightChecker(cityPath, scopeRoot string) contract.PreflightChecker {
	return contract.PreflightChecker{
		FS:                fsys.OSFS{},
		Provider:          rawBeadsProviderForScope(scopeRoot, cityPath),
		BDContext:         preflightBDContextReader(cityPath),
		DatabaseProjectID: preflightDatabaseProjectIDReader(cityPath),
	}
}

func preflightBDContextReader(cityPath string) func(scope string) (contract.PreflightBDContext, error) {
	return func(scope string) (contract.PreflightBDContext, error) {
		out, err := bdCommandRunnerForCity(cityPath)(scope, "bd", "context", "--json")
		if err != nil {
			return contract.PreflightBDContext{}, err
		}
		var raw struct {
			Backend  string `json:"backend"`
			DoltMode string `json:"dolt_mode"`
		}
		if err := json.Unmarshal(out, &raw); err != nil {
			return contract.PreflightBDContext{}, fmt.Errorf("parse bd context --json: %w", err)
		}
		return contract.PreflightBDContext{
			Backend:  raw.Backend,
			DoltMode: raw.DoltMode,
		}, nil
	}
}

func preflightDatabaseProjectIDReader(cityPath string) func(scope string) (string, bool, error) {
	return func(scope string) (string, bool, error) {
		target, ok, err := canonicalScopeDoltTarget(cityPath, scope)
		if err != nil || !ok {
			return "", false, err
		}
		db, err := managedDoltOpenDatabase(target.Host, target.Port, target.User, target.Database)
		if err != nil {
			return "", false, err
		}
		defer db.Close() //nolint:errcheck // read-only best-effort close

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			return "", false, err
		}
		return readDatabaseProjectID(ctx, db)
	}
}

func doBeadsPreflight(opts beadsPreflightOptions, check beadsPreflightCheckFunc, stdout, stderr io.Writer) int {
	scope := strings.TrimSpace(opts.Scope)
	if scope == "" {
		fmt.Fprintln(stderr, "gc beads preflight: missing scope") //nolint:errcheck // best-effort stderr
		return 3
	}
	if check == nil {
		fmt.Fprintln(stderr, "gc beads preflight: preflight checker is not configured") //nolint:errcheck // best-effort stderr
		return 3
	}
	result, err := check(scope)
	if err != nil {
		fmt.Fprintf(stderr, "gc beads preflight: %v\n", err) //nolint:errcheck // best-effort stderr
		return 3
	}
	result = contract.NewPreflightResult(result)
	if result.Scope == "" {
		result.Scope = scope
	}
	if opts.JSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fmt.Fprintf(stderr, "gc beads preflight: write JSON: %v\n", err) //nolint:errcheck // best-effort stderr
			return 3
		}
		return beadsPreflightExitCode(result.Verdict)
	}
	renderBeadsPreflightHuman(stdout, result, opts.Verbose)
	return beadsPreflightExitCode(result.Verdict)
}

func renderBeadsPreflightHuman(w io.Writer, result contract.PreflightResult, verbose bool) {
	fmt.Fprintln(w, "Beads backend preflight")      //nolint:errcheck // best-effort stdout
	fmt.Fprintf(w, "  Scope: %s\n\n", result.Scope) //nolint:errcheck // best-effort stdout
	for _, check := range result.Checks {
		fmt.Fprintf(w, "  [%s] %-22s %s\n", check.State, preflightCheckDisplayName(check.ID), check.Summary) //nolint:errcheck // best-effort stdout
		if verbose {
			renderBeadsPreflightDetails(w, check.Details)
		}
	}
	fmt.Fprintf(w, "\n  Verdict: %s\n", result.Verdict) //nolint:errcheck // best-effort stdout
	if result.NativeStoreEligible {
		fmt.Fprintln(w, "  Native store is eligible.") //nolint:errcheck // best-effort stdout
	} else if result.Fallback != contract.PreflightFallbackNone {
		fmt.Fprintf(w, "  Native store is disabled. Using %s fallback.\n", result.Fallback) //nolint:errcheck // best-effort stdout
	}
	renderBeadsPreflightRepairGuide(w, result.RepairSteps)
}

func renderBeadsPreflightDetails(w io.Writer, details contract.PreflightDetails) {
	writeDetail := func(name, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			fmt.Fprintf(w, "                              %s=%s\n", name, value) //nolint:errcheck // best-effort stdout
		}
	}
	writeBoolDetail := func(name string, value *bool) {
		if value != nil {
			fmt.Fprintf(w, "                              %s=%t\n", name, *value) //nolint:errcheck // best-effort stdout
		}
	}
	writeDetail("provider", details.Provider)
	writeDetail("metadata_backend", details.MetadataBackend)
	writeDetail("bd_context_backend", details.BDContextBackend)
	writeDetail("bd_context_dolt_mode", details.BDContextDoltMode)
	writeBoolDetail("has_postgres_dsn", details.HasPostgresDSN)
	writeBoolDetail("has_split_fields", details.HasSplitFields)
	writeDetail("postgres_dsn", details.PostgresDSNRedacted)
	writeDetail("postgres_host", details.PostgresHost)
	writeDetail("postgres_port", details.PostgresPort)
	writeDetail("postgres_user", details.PostgresUser)
	writeDetail("postgres_database", details.PostgresDatabase)
	writeDetail("metadata_project_id", details.MetadataProjectID)
	writeDetail("db_project_id", details.DBProjectID)
	writeDetail("expected", details.Expected)
	for _, field := range details.AdditionalDiagnostics {
		writeDetail(field.Key, field.Value)
	}
}

func renderBeadsPreflightRepairGuide(w io.Writer, steps []contract.PreflightRepairStep) {
	if len(steps) == 0 {
		return
	}
	fmt.Fprintln(w, "\n  Repair guide") //nolint:errcheck // best-effort stdout
	for _, step := range steps {
		fmt.Fprintf(w, "  [%s:%s]\n", step.Priority, step.CheckID) //nolint:errcheck // best-effort stdout
		if strings.TrimSpace(step.Command) != "" {
			fmt.Fprintf(w, "    Run: %s\n", step.Command) //nolint:errcheck // best-effort stdout
		}
		if strings.TrimSpace(step.Note) != "" {
			fmt.Fprintf(w, "    %s\n", step.Note) //nolint:errcheck // best-effort stdout
		}
	}
}

func preflightCheckDisplayName(id contract.PreflightCheckID) string {
	switch id {
	case contract.PreflightCheckProviderContract:
		return "Provider contract"
	case contract.PreflightCheckMetadataBackend:
		return "Metadata backend"
	case contract.PreflightCheckBDContextAgreement:
		return "BD context agreement"
	case contract.PreflightCheckIdentityMatch:
		return "Identity match"
	case contract.PreflightCheckContractShape:
		return "Contract shape"
	default:
		return string(id)
	}
}

func beadsPreflightExitCode(verdict contract.PreflightVerdict) int {
	switch verdict {
	case contract.PreflightVerdictEligible:
		return 0
	case contract.PreflightVerdictBlocked:
		return 1
	case contract.PreflightVerdictDegraded:
		return 2
	default:
		return 3
	}
}
