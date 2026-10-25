//go:build integration

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A real child process owns argv/output behavior; no production executor is replaced.
func installDataReadDoltCLI(t *testing.T, csv, listErr, readErr string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	script := `#!/bin/sh
printf '%s\n' '--call--' "$@" >> "$PROBE_TEST_CALLS"
query=''
while [ "$#" -gt 0 ]; do
    if [ "$1" = '-q' ]; then shift; query="$1"; fi
    shift
done
case "$query" in
    *information_schema.tables*)
        if [ -n "$PROBE_TEST_LIST_ERROR" ]; then printf '%s\n' "$PROBE_TEST_LIST_ERROR" >&2; exit 1; fi
        printf '%s' "$PROBE_TEST_CSV"
        ;;
    *)
        if [ -n "$PROBE_TEST_READ_ERROR" ]; then printf '%s\n' "$PROBE_TEST_READ_ERROR" >&2; exit 1; fi
        ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "dolt"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GC_DOLT_PASSWORD", "")
	t.Setenv("PROBE_TEST_CALLS", log)
	t.Setenv("PROBE_TEST_CSV", csv)
	t.Setenv("PROBE_TEST_LIST_ERROR", listErr)
	t.Setenv("PROBE_TEST_READ_ERROR", readErr)
	return log
}

func TestManagedDoltDataReadCLIArgumentsAndBatching(t *testing.T) {
	log := installDataReadDoltCLI(t, "table_schema,table_name\nqa-db,wisps\nqa-db,events\nother,leases\n", "", "")
	got := managedDoltDataReadProbe("127.0.0.1", "3307", "probe-user")
	if got != (managedDoltDataReadResult{}) {
		t.Fatalf("probe = %+v, want healthy", got)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	calls := strings.Split(strings.TrimPrefix(string(data), "--call--\n"), "--call--\n")
	if len(calls) != 3 {
		t.Fatalf("calls = %q, want discovery and one read per database", calls)
	}
	base := []string{"--host", "127.0.0.1", "--port", "3307", "--user", "probe-user", "--password", "", "--no-tls", "sql"}
	for i, call := range calls {
		args := strings.Split(strings.TrimSuffix(call, "\n"), "\n")
		if len(args) < len(base) || !reflect.DeepEqual(args[:len(base)], base) {
			t.Fatalf("call %d endpoint/auth args = %q, want prefix %q", i, args, base)
		}
		args = args[len(base):]
		if i == 0 {
			if len(args) != 4 || !reflect.DeepEqual(args[:3], []string{"-r", "csv", "-q"}) || !strings.Contains(args[3], "information_schema.tables") {
				t.Fatalf("discovery args = %q, want CSV catalog query", args)
			}
			continue
		}
		wantSQL := "SELECT * FROM `qa-db`.`wisps` LIMIT 1; SELECT * FROM `qa-db`.`events` LIMIT 1;"
		if i == 2 {
			wantSQL = "SELECT * FROM `other`.`leases` LIMIT 1;"
		}
		if !reflect.DeepEqual(args, []string{"-q", wantSQL}) {
			t.Fatalf("read args = %q, want one joined batch %q", args, wantSQL)
		}
	}
}

func TestManagedDoltDataReadCLIPropagatesAdapterErrors(t *testing.T) {
	for _, tc := range []struct {
		name, csv, listErr, readErr, want string
		chunk                             bool
	}{
		{name: "discovery process", listErr: "catalog permission denied", want: "catalog permission denied"},
		{name: "discovery CSV", csv: "table_schema,table_name\nqa\n", want: "parse dolt-ignored table list csv"},
		{name: "chunk read", csv: "table_schema,table_name\nqa,wisps\n", readErr: testEmptyChunkErr, want: testEmptyChunkErr, chunk: true},
		{name: "transient read", csv: "table_schema,table_name\nqa,wisps\n", readErr: "connection reset", want: "connection reset"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			installDataReadDoltCLI(t, tc.csv, tc.listErr, tc.readErr)
			got := managedDoltDataReadProbe("127.0.0.1", "3307", "root")
			if tc.chunk {
				if !strings.Contains(got.Err, tc.want) || got.Warning != "" {
					t.Fatalf("probe = %+v, want chunk failure %q", got, tc.want)
				}
			} else if got.Err != "" || !strings.Contains(got.Warning, tc.want) {
				t.Fatalf("probe = %+v, want warning %q", got, tc.want)
			}
		})
	}
}

func TestManagedDoltDataReadCLICancelledBeforeSpawn(t *testing.T) {
	log := installDataReadDoltCLI(t, "table_schema,table_name\nqa,wisps\n", "", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ops := managedDoltDataReadOpsCLI(ctx, "127.0.0.1", "3307", "root")
	if _, err := ops.listTables(); !errors.Is(err, context.Canceled) {
		t.Fatalf("listTables error = %v, want cancellation", err)
	}
	if err := ops.readBatch("qa", []string{"SELECT * FROM `qa`.`wisps` LIMIT 1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("readBatch error = %v, want cancellation", err)
	}
	if _, err := os.Stat(log); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled adapter spawned dolt: log stat = %v", err)
	}
}
