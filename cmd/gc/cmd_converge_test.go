package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/convergence"
	"github.com/spf13/cobra"
)

func TestConvergeCreateSendsRigParam(t *testing.T) {
	resetFlags(t)
	t.Setenv("GC_RIG", "")

	cityPath := setupCity(t, "converge-create-rig")
	var gotCityPath string
	var gotReq convergenceRequest
	oldSender := sendConvergenceRequestForCLI
	sendConvergenceRequestForCLI = func(cityPath string, req convergenceRequest) (convergenceReply, error) {
		gotCityPath = cityPath
		gotReq = req
		result, _ := json.Marshal(convergence.CreateResult{BeadID: "gc-converge-1"})
		return convergenceReply{Result: result}, nil
	}
	t.Cleanup(func() { sendConvergenceRequestForCLI = oldSender })

	var stdout, stderr bytes.Buffer
	cmd := newRootCmd(&stdout, &stderr)
	cmd.SetArgs([]string{
		"--city", cityPath,
		"converge", "create",
		"--formula", "mol-tdd-build",
		"--target", "agent-under-test",
		"--rig", "rig-under-test",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v; stderr=%q", err, stderr.String())
	}
	if gotCityPath != cityPath {
		t.Fatalf("cityPath = %q, want %q", gotCityPath, cityPath)
	}
	if gotReq.Params["rig"] != "rig-under-test" {
		t.Fatalf("request rig param = %q, want rig-under-test", gotReq.Params["rig"])
	}
	if got := strings.TrimSpace(stdout.String()); got != "gc-converge-1" {
		t.Fatalf("stdout = %q, want bead id", got)
	}
}

func TestConvergeListAggregatesStoresWithStoreColumn(t *testing.T) {
	cityStore := beads.NewMemStore()
	rigStore := beads.NewMemStore()
	addConvergenceListBead(t, cityStore, "city loop", "city-agent")
	addConvergenceListBead(t, rigStore, "rig loop", "rig-agent")
	stubConvergeStores(t, []convergeStoreRef{
		{Key: "", Store: cityStore},
		{Key: "rig-a", Store: rigStore},
	})

	var stdout, stderr bytes.Buffer
	cmd := newConvergeListCmd(&stdout, &stderr)
	silenceCobraUsage(cmd)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v; stderr=%q", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "ID             STORE") {
		t.Fatalf("list output missing STORE column: %q", out)
	}
	if !strings.Contains(out, "city") || !strings.Contains(out, "rig-a") {
		t.Fatalf("list output missing store names: %q", out)
	}
}

func TestConvergeListRigSuppressesStoreColumn(t *testing.T) {
	rigStore := beads.NewMemStore()
	addConvergenceListBead(t, rigStore, "rig loop", "rig-agent")
	stubConvergeStores(t, []convergeStoreRef{{Key: "rig-a", Store: rigStore}})

	var stdout, stderr bytes.Buffer
	cmd := newConvergeListCmd(&stdout, &stderr)
	silenceCobraUsage(cmd)
	cmd.SetArgs([]string{"--rig", "rig-a"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v; stderr=%q", err, stderr.String())
	}
	header := strings.SplitN(stdout.String(), "\n", 2)[0]
	if strings.Contains(header, "STORE") {
		t.Fatalf("narrowed list header = %q, want no STORE column", header)
	}
	if !strings.Contains(stdout.String(), "rig loop") {
		t.Fatalf("list output missing rig row: %q", stdout.String())
	}
}

func TestConvergeListJSONIncludesStore(t *testing.T) {
	cityStore := beads.NewMemStore()
	addConvergenceListBead(t, cityStore, "city loop", "city-agent")
	stubConvergeStores(t, []convergeStoreRef{{Key: "", Store: cityStore}})

	var stdout, stderr bytes.Buffer
	cmd := newConvergeListCmd(&stdout, &stderr)
	silenceCobraUsage(cmd)
	cmd.SetArgs([]string{"--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v; stderr=%q", err, stderr.String())
	}
	var rows []struct {
		Store string `json:"store"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatalf("unmarshal list json: %v; output=%q", err, stdout.String())
	}
	if len(rows) != 1 || rows[0].Store != "city" {
		t.Fatalf("rows = %#v, want one city row", rows)
	}
}

func TestConvergeStatusFindsRigStoreByBeadID(t *testing.T) {
	rigStore := beads.NewMemStore()
	b := addConvergenceListBead(t, rigStore, "rig loop", "rig-agent")
	stubConvergeStores(t, []convergeStoreRef{{Key: "rig-a", Store: rigStore}})

	var stdout, stderr bytes.Buffer
	cmd := newConvergeStatusCmd(&stdout, &stderr)
	silenceCobraUsage(cmd)
	cmd.SetArgs([]string{b.ID})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v; stderr=%q", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Store:           rig-a") || !strings.Contains(out, "Title:           rig loop") {
		t.Fatalf("status output did not report rig store: %q", out)
	}
}

func TestConvergeTestGateFindsRigStoreByBeadID(t *testing.T) {
	rigStore := beads.NewMemStore()
	b := addConvergenceListBead(t, rigStore, "rig loop", "rig-agent")
	stubConvergeStores(t, []convergeStoreRef{{Key: "rig-a", Store: rigStore}})

	var stdout, stderr bytes.Buffer
	cmd := newConvergeTestGateCmd(&stdout, &stderr)
	silenceCobraUsage(cmd)
	cmd.SetArgs([]string{b.ID})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v; stderr=%q", err, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "Gate mode is manual — no condition to test." {
		t.Fatalf("stdout = %q, want manual gate message", got)
	}
}

func silenceCobraUsage(cmd *cobra.Command) {
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
}

func stubConvergeStores(t *testing.T, stores []convergeStoreRef) {
	t.Helper()
	oldOpen := openConvergeStoresForCLI
	openConvergeStoresForCLI = func(_ io.Writer, _ string, rigFilter *string) ([]convergeStoreRef, int) {
		if rigFilter == nil {
			return stores, 0
		}
		var filtered []convergeStoreRef
		for _, ref := range stores {
			if ref.Key == *rigFilter || (*rigFilter == "" && ref.Key == "") {
				filtered = append(filtered, ref)
			}
		}
		return filtered, 0
	}
	t.Cleanup(func() { openConvergeStoresForCLI = oldOpen })
}

func addConvergenceListBead(t *testing.T, store *beads.MemStore, title, target string) beads.Bead {
	t.Helper()
	b, err := store.Create(beads.Bead{
		Title: title,
		Type:  "convergence",
	})
	if err != nil {
		t.Fatalf("create bead: %v", err)
	}
	updates := map[string]string{
		convergence.FieldState:         convergence.StateActive,
		convergence.FieldIteration:     "1",
		convergence.FieldMaxIterations: "3",
		convergence.FieldGateMode:      convergence.GateModeManual,
		convergence.FieldFormula:       "mol-tdd-build",
		convergence.FieldTarget:        target,
	}
	for key, value := range updates {
		if err := store.SetMetadata(b.ID, key, value); err != nil {
			t.Fatalf("set metadata %s: %v", key, err)
		}
	}
	updated, err := store.Get(b.ID)
	if err != nil {
		t.Fatalf("get bead: %v", err)
	}
	return updated
}
