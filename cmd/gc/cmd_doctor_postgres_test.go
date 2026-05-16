package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads/contract"
	"github.com/gastownhall/gascity/internal/fsys"
)

func TestPostgresChecksRegistrationOrder(t *testing.T) {
	host, port := listenDoctorPostgresServer(t)
	cityDir := newDoctorPostgresCity(t, "postgres", host, port)
	withDoctorCity(t, cityDir)

	var stdout, stderr bytes.Buffer
	_ = doDoctor(false, false, false, &stdout, &stderr)
	out := stdout.String() + stderr.String()
	serverIdx := strings.Index(out, "postgres-server")
	authIdx := strings.Index(out, "postgres-auth")
	if serverIdx < 0 {
		t.Fatalf("doctor output missing postgres-server:\n%s", out)
	}
	if authIdx < 0 {
		t.Fatalf("doctor output missing postgres-auth:\n%s", out)
	}
	if serverIdx > authIdx {
		t.Fatalf("postgres-server index %d after postgres-auth index %d:\n%s", serverIdx, authIdx, out)
	}
}

func TestPostgresChecksNotRegisteredForPureDoltCity(t *testing.T) {
	cityDir := newDoctorPostgresCity(t, "dolt", "127.0.0.1", "5432")
	withDoctorCity(t, cityDir)

	var stdout, stderr bytes.Buffer
	_ = doDoctor(false, false, false, &stdout, &stderr)
	out := stdout.String() + stderr.String()
	if strings.Contains(out, "postgres-server") {
		t.Fatalf("doctor output contains postgres-server for pure Dolt city:\n%s", out)
	}
	if strings.Contains(out, "postgres-auth") {
		t.Fatalf("doctor output contains postgres-auth for pure Dolt city:\n%s", out)
	}
}

func TestCityHasPostgresScopeDefinedExactlyOnce(t *testing.T) {
	if cityHasPostgresScope(t.TempDir(), nil) {
		t.Fatal("cityHasPostgresScope() = true, want false for nil config")
	}
	matches := grepFuncDefinitions(t, "func cityHasPostgresScope(")
	if len(matches) != 1 {
		t.Fatalf("cityHasPostgresScope must be defined exactly once; got %d matches: %v", len(matches), matches)
	}
}

func newDoctorPostgresCity(t *testing.T, backend, host, port string) string {
	t.Helper()
	cityDir := t.TempDir()
	rigDir := filepath.Join(cityDir, "rigs", "frontend")
	if err := os.MkdirAll(filepath.Join(rigDir, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cityDir, "city.toml"), []byte(`[workspace]
name = "demo"

[beads]
provider = "file"

[[rigs]]
name = "frontend"
path = "rigs/frontend"
prefix = "fe"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if backend == "postgres" {
		if _, err := contract.EnsureCanonicalMetadata(fsys.OSFS{}, filepath.Join(rigDir, ".beads", "metadata.json"), contract.MetadataState{
			Database:         "postgres",
			Backend:          "postgres",
			PostgresHost:     host,
			PostgresPort:     port,
			PostgresUser:     "bd",
			PostgresDatabase: "beads_pg",
		}); err != nil {
			t.Fatal(err)
		}
		return cityDir
	}
	if _, err := contract.EnsureCanonicalMetadata(fsys.OSFS{}, filepath.Join(rigDir, ".beads", "metadata.json"), contract.MetadataState{
		Database:     "dolt",
		Backend:      "dolt",
		DoltMode:     "server",
		DoltDatabase: "fe",
	}); err != nil {
		t.Fatal(err)
	}
	return cityDir
}

func listenDoctorPostgresServer(t *testing.T) (string, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

func withDoctorCity(t *testing.T, cityDir string) {
	t.Helper()
	oldCityFlag := cityFlag
	cityFlag = cityDir
	t.Setenv("GC_CITY_PATH", cityDir)
	t.Cleanup(func() { cityFlag = oldCityFlag })
}

func grepFuncDefinitions(t *testing.T, needle string) []string {
	t.Helper()
	var matches []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		scanner := bufio.NewScanner(f)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			if strings.Contains(scanner.Text(), needle) {
				matches = append(matches, fmt.Sprintf("%s:%d", path, lineNo))
			}
		}
		return scanner.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return matches
}
