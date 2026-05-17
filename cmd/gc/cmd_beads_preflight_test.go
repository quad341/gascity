package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads/contract"
)

func TestNewBeadsCmdIncludesPreflight(t *testing.T) {
	cmd := newBeadsCmd(&bytes.Buffer{}, &bytes.Buffer{})
	preflight, _, err := cmd.Find([]string{"preflight"})
	if err != nil {
		t.Fatalf("Find(preflight): %v", err)
	}
	if preflight == nil || preflight.Name() != "preflight" {
		t.Fatalf("preflight command = %#v", preflight)
	}
}

func TestDoBeadsPreflightHumanBlocked(t *testing.T) {
	result := contract.NewPreflightResult(contract.PreflightResult{
		Verdict: contract.PreflightVerdictBlocked,
		Scope:   "/city",
		Checks: []contract.PreflightCheckResult{
			contract.NewPreflightCheckResult(contract.PreflightCheckProviderContract, contract.PreflightCheckPass, "Provider exposes bd contract", contract.PreflightDetails{Provider: "bd"}),
			contract.NewPreflightCheckResult(contract.PreflightCheckMetadataBackend, contract.PreflightCheckFail, "Metadata backend is postgres", contract.PreflightDetails{MetadataBackend: "postgres"}),
			contract.NewPreflightCheckResult(contract.PreflightCheckContractShape, contract.PreflightCheckWarn, "postgres_dsn present; Gas City expects split fields", contract.PreflightDetails{
				HasPostgresDSN:      boolPtr(true),
				PostgresDSNRedacted: "postgres://operator:swordfish@db.example.com/gascity",
			}),
		},
		RepairSteps: []contract.PreflightRepairStep{{
			CheckID:  contract.PreflightCheckIdentityMatch,
			Priority: contract.PreflightRepairCritical,
			Command:  "bd doctor --fix",
			Note:     "Identity mismatch is the highest-severity failure.",
		}},
		NativeStoreEligible: false,
		Fallback:            contract.PreflightFallbackBdStore,
	})

	var stdout, stderr bytes.Buffer
	code := doBeadsPreflight(beadsPreflightOptions{Scope: "/city", Verbose: true}, func(scope string) (contract.PreflightResult, error) {
		if scope != "/city" {
			t.Fatalf("scope = %q, want /city", scope)
		}
		return result, nil
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doBeadsPreflight() = %d, want 1; stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"Beads backend preflight",
		"Scope: /city",
		"[PASS]",
		"[WARN]",
		"[FAIL]",
		"Verdict: BLOCKED",
		"Using BdStore fallback",
		"Repair guide",
		"bd doctor --fix",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "swordfish") {
		t.Fatalf("human output leaked DSN secret:\n%s", out)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestDoBeadsPreflightHumanRepairGuideUsesPriorityAndCategories(t *testing.T) {
	result := contract.NewPreflightResult(contract.PreflightResult{
		Verdict: contract.PreflightVerdictBlocked,
		Scope:   "/city",
		Checks: []contract.PreflightCheckResult{
			contract.NewPreflightCheckResult(contract.PreflightCheckMetadataBackend, contract.PreflightCheckWarn, "Metadata backend is postgres (postgres_dsn form)", contract.PreflightDetails{MetadataBackend: "postgres"}),
			contract.NewPreflightCheckResult(contract.PreflightCheckBDContextAgreement, contract.PreflightCheckFail, "Metadata backend=postgres; bd context reports backend=dolt", contract.PreflightDetails{MetadataBackend: "postgres", BDContextBackend: "dolt"}),
			contract.NewPreflightCheckResult(contract.PreflightCheckIdentityMatch, contract.PreflightCheckFail, "project_id mismatch", contract.PreflightDetails{MetadataProjectID: "metadata-id", DBProjectID: "database-id"}),
			contract.NewPreflightCheckResult(contract.PreflightCheckContractShape, contract.PreflightCheckWarn, "postgres_dsn present; Gas City expects split fields", contract.PreflightDetails{HasPostgresDSN: boolPtr(true)}),
		},
		RepairSteps: []contract.PreflightRepairStep{
			{CheckID: contract.PreflightCheckMetadataBackend, Priority: contract.PreflightRepairRecommended, Command: "bd bootstrap"},
			{CheckID: contract.PreflightCheckBDContextAgreement, Priority: contract.PreflightRepairRecommended, Command: "bd context --json"},
			{CheckID: contract.PreflightCheckIdentityMatch, Priority: contract.PreflightRepairCritical, Command: "bd doctor --fix"},
			{CheckID: contract.PreflightCheckContractShape, Priority: contract.PreflightRepairRecommended, Command: "bd bootstrap"},
		},
		NativeStoreEligible: false,
		Fallback:            contract.PreflightFallbackBdStore,
	})

	var stdout, stderr bytes.Buffer
	code := doBeadsPreflight(beadsPreflightOptions{Scope: "/city"}, func(string) (contract.PreflightResult, error) {
		return result, nil
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doBeadsPreflight() = %d, want 1; stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"[critical:identity-mismatch]",
		"[recommended:metadata-backend]",
		"[recommended:bd-context-agreement]",
		"[recommended:contract-shape]",
		"Run: bd doctor --fix",
		"Run: bd bootstrap",
		"Run: bd context --json",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "[critical:identity-mismatch]") > strings.Index(out, "[recommended:metadata-backend]") {
		t.Fatalf("critical identity guidance should be prioritized before recommended metadata guidance:\n%s", out)
	}
}

func TestDoBeadsPreflightJSONRedactedDegraded(t *testing.T) {
	result := contract.PreflightResult{
		Verdict: contract.PreflightVerdictDegraded,
		Scope:   "/city",
		Checks: []contract.PreflightCheckResult{
			{
				ID:      contract.PreflightCheckContractShape,
				State:   contract.PreflightCheckWarn,
				Summary: "postgres_dsn present; Gas City expects split fields",
				Details: contract.PreflightDetails{
					PostgresDSNRedacted: "postgres://operator:swordfish@db.example.com/gascity",
				},
			},
		},
		NativeStoreEligible: false,
		Fallback:            contract.PreflightFallbackBdStore,
	}

	var stdout, stderr bytes.Buffer
	code := doBeadsPreflight(beadsPreflightOptions{Scope: "/city", JSON: true}, func(string) (contract.PreflightResult, error) {
		return result, nil
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("doBeadsPreflight() = %d, want 2; stderr=%s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "swordfish") {
		t.Fatalf("JSON output leaked DSN secret:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "postgres://[REDACTED]") {
		t.Fatalf("JSON output missing redacted DSN:\n%s", stdout.String())
	}
	var decoded contract.PreflightResult
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("JSON output did not decode: %v\n%s", err, stdout.String())
	}
	if decoded.Verdict != contract.PreflightVerdictDegraded || decoded.NativeStoreEligible {
		t.Fatalf("decoded result = %+v, want degraded and native-store ineligible", decoded)
	}
}

func TestDoBeadsPreflightExitCodes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		verdict contract.PreflightVerdict
		err     error
		want    int
	}{
		{name: "eligible", verdict: contract.PreflightVerdictEligible, want: 0},
		{name: "blocked", verdict: contract.PreflightVerdictBlocked, want: 1},
		{name: "degraded", verdict: contract.PreflightVerdictDegraded, want: 2},
		{name: "unable", err: errors.New("metadata unreadable"), want: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := doBeadsPreflight(beadsPreflightOptions{Scope: "/city"}, func(string) (contract.PreflightResult, error) {
				if tc.err != nil {
					return contract.PreflightResult{}, tc.err
				}
				return contract.PreflightResult{Verdict: tc.verdict, Scope: "/city"}, nil
			}, &stdout, &stderr)
			if code != tc.want {
				t.Fatalf("doBeadsPreflight() = %d, want %d; stdout=%s stderr=%s", code, tc.want, stdout.String(), stderr.String())
			}
			if tc.err != nil && !strings.Contains(stderr.String(), "metadata unreadable") {
				t.Fatalf("stderr = %q, want unable-to-run error", stderr.String())
			}
		})
	}
}

func TestNewBeadsPreflightHelpDocumentsContract(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := newBeadsPreflightCmd(&stdout, &stderr)
	cmd.SetArgs([]string{"--help"})
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(--help): %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"--scope",
		"--json",
		"--verbose",
		"ELIGIBLE=0",
		"BLOCKED=1",
		"DEGRADED=2",
		"unable-to-run=3",
		"no --fix",
		"operator",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q:\n%s", want, out)
		}
	}
}
