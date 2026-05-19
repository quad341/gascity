package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestConvergeCreateRejectsRigFlagBeforeSocketWork(t *testing.T) {
	resetFlags(t)
	t.Setenv("GC_RIG", "")

	cityPath := setupCity(t, "converge-create-rig")
	var stdout, stderr bytes.Buffer
	cmd := newRootCmd(&stdout, &stderr)
	cmd.SetArgs([]string{
		"--city", cityPath,
		"--rig", "rig-under-test",
		"converge", "create",
		"--formula", "mol-tdd-build",
		"--target", "agent-under-test",
	})

	err := cmd.Execute()
	requireConvergeRigUnsupported(t, err, stderr.String(), "create")
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

func TestConvergeListRejectsGCRigBeforeStoreWork(t *testing.T) {
	resetFlags(t)
	t.Setenv("GC_RIG", "rig-under-test")
	t.Setenv("GC_BEADS", "file")
	cityFlag = setupCity(t, "converge-list-rig")

	var stdout, stderr bytes.Buffer
	cmd := newConvergeListCmd(&stdout, &stderr)
	silenceCobraUsage(cmd)

	err := cmd.Execute()
	requireConvergeRigUnsupported(t, err, stderr.String(), "list")
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

func TestConvergeStatusRejectsRigFlagBeforeStoreWork(t *testing.T) {
	resetFlags(t)
	t.Setenv("GC_RIG", "")
	t.Setenv("GC_BEADS", "file")
	cityFlag = setupCity(t, "converge-status-rig")
	rigFlag = "rig-under-test"

	var stdout, stderr bytes.Buffer
	cmd := newConvergeStatusCmd(&stdout, &stderr)
	silenceCobraUsage(cmd)
	cmd.SetArgs([]string{"ga-converge-1"})

	err := cmd.Execute()
	requireConvergeRigUnsupported(t, err, stderr.String(), "status")
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

func TestConvergeTestGateRejectsGCRigBeforeStoreWork(t *testing.T) {
	resetFlags(t)
	t.Setenv("GC_RIG", "rig-under-test")
	t.Setenv("GC_BEADS", "file")
	cityFlag = setupCity(t, "converge-test-gate-rig")

	var stdout, stderr bytes.Buffer
	cmd := newConvergeTestGateCmd(&stdout, &stderr)
	silenceCobraUsage(cmd)
	cmd.SetArgs([]string{"ga-converge-1"})

	err := cmd.Execute()
	requireConvergeRigUnsupported(t, err, stderr.String(), "test-gate")
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

func TestConvergeSocketCommandsRejectRigFlagBeforeDial(t *testing.T) {
	for _, command := range []string{"approve", "iterate", "stop"} {
		t.Run(command, func(t *testing.T) {
			resetFlags(t)
			t.Setenv("GC_RIG", "")
			cityFlag = setupCity(t, "converge-"+command+"-rig")
			rigFlag = "rig-under-test"

			var stdout, stderr bytes.Buffer
			err := convergeSocketCmd("ga-converge-1", command, nil, &stdout, &stderr)

			requireConvergeRigUnsupported(t, err, stderr.String(), command)
			if got := stdout.String(); got != "" {
				t.Fatalf("stdout = %q, want empty", got)
			}
		})
	}
}

func TestConvergeRetryRejectsRigFlagBeforeSocketWork(t *testing.T) {
	resetFlags(t)
	t.Setenv("GC_RIG", "")

	cityPath := setupCity(t, "converge-retry-rig")
	var stdout, stderr bytes.Buffer
	cmd := newRootCmd(&stdout, &stderr)
	cmd.SetArgs([]string{
		"--city", cityPath,
		"--rig", "rig-under-test",
		"converge", "retry", "ga-converge-1",
	})

	err := cmd.Execute()
	requireConvergeRigUnsupported(t, err, stderr.String(), "retry")
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

func silenceCobraUsage(cmd *cobra.Command) {
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
}

func requireConvergeRigUnsupported(t *testing.T, err error, stderr, command string) {
	t.Helper()
	if !errors.Is(err, errExit) {
		t.Fatalf("error = %v, want errExit", err)
	}
	want := convergeRigUnsupportedMessage(command)
	if got := stderr; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func convergeRigUnsupportedMessage(command string) string {
	return strings.Join([]string{
		"gc converge " + command + ": --rig is not supported; convergence loops are city-scoped.",
		"Use a city-scoped formula whose wisps target rig-bound agents; the wisp executes inside the rig even though the root bead lives in city/HQ.",
		"",
	}, "\n")
}
