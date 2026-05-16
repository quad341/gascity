package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads/contract"
	"github.com/gastownhall/gascity/internal/config"
)

func TestPostgresServerCheck(t *testing.T) {
	t.Run("HappyPath_OneScope_DialSucceeds", func(t *testing.T) {
		host, port := listenPostgresServer(t)
		withSystemdUserLinger(t, "linux", true, true, nil)
		cityPath, cfg := newPostgresServerCity(t, postgresServerScopeSpec{
			kind: "rig",
			name: "frontend",
			path: "rigs/frontend",
			host: host,
			port: port,
		})

		result := NewPostgresServerCheck(cityPath, cfg).Run(&CheckContext{CityPath: cityPath})
		if result.Status != StatusOK {
			t.Fatalf("status = %v, want OK; result = %+v", result.Status, result)
		}
		if got, want := result.Message, "reachable at "+net.JoinHostPort(host, port); got != want {
			t.Fatalf("message = %q, want %q", got, want)
		}
		if result.FixHint != "" {
			t.Fatalf("fix hint = %q, want empty", result.FixHint)
		}
		if len(result.Details) != 0 {
			t.Fatalf("non-verbose details = %v, want empty", result.Details)
		}

		verbose := NewPostgresServerCheck(cityPath, cfg).Run(&CheckContext{CityPath: cityPath, Verbose: true})
		wantDetails := []string{"✓ rigs/frontend (" + host + ":" + port + ") — reachable at " + net.JoinHostPort(host, port)}
		if !reflect.DeepEqual(verbose.Details, wantDetails) {
			t.Fatalf("verbose details = %#v, want %#v", verbose.Details, wantDetails)
		}
	})

	for _, tc := range []struct {
		name string
		goos string
		want string
	}{
		{
			name: "SingleScope_DialFails_LinuxFixHint",
			goos: "linux",
			want: "start PG (e.g. systemctl --user start postgresql, sudo systemctl start postgresql, or docker compose up -d postgres) then re-run gc doctor",
		},
		{
			name: "SingleScope_DialFails_DarwinFixHint",
			goos: "darwin",
			want: "start PG (e.g. brew services start postgresql@<version>, launch Postgres.app, or docker compose up -d postgres) then re-run gc doctor",
		},
		{
			name: "SingleScope_DialFails_WindowsFixHint",
			goos: "windows",
			want: "start the PostgreSQL service (services.msc → PostgreSQL → Start, or pg_ctl start -D <data-dir>) then re-run gc doctor",
		},
		{
			name: "SingleScope_DialFails_DefaultFixHint",
			goos: "plan9",
			want: "gc does not manage external PostgreSQL servers; start it via your OS supervisor or container runtime, then re-run gc doctor",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withPostgresServerDialResults(t, map[string]error{
				net.JoinHostPort("127.0.0.1", "55432"): errors.New("connection refused"),
			})
			withSystemdUserLinger(t, tc.goos, true, true, nil)
			cityPath, cfg := newPostgresServerCity(t, postgresServerScopeSpec{
				kind: "rig",
				name: "frontend",
				path: "rigs/frontend",
				host: "127.0.0.1",
				port: "55432",
			})

			result := NewPostgresServerCheck(cityPath, cfg).Run(&CheckContext{CityPath: cityPath})
			if result.Status != StatusError {
				t.Fatalf("status = %v, want Error; result = %+v", result.Status, result)
			}
			if got, want := result.Message, "server not reachable at 127.0.0.1:55432"; got != want {
				t.Fatalf("message = %q, want %q", got, want)
			}
			if result.FixHint != tc.want {
				t.Fatalf("fix hint = %q, want %q", result.FixHint, tc.want)
			}
		})
	}

	t.Run("SingleScope_DialFails_RemoteHost_AppendsCloudHint", func(t *testing.T) {
		withPostgresServerDialResults(t, map[string]error{
			net.JoinHostPort("db.example.com", "5432"): errors.New("connection refused"),
		})
		withSystemdUserLinger(t, "linux", true, true, nil)
		cityPath, cfg := newPostgresServerCity(t, postgresServerScopeSpec{
			kind: "rig",
			name: "frontend",
			path: "rigs/frontend",
			host: "db.example.com",
			port: "5432",
		})

		result := NewPostgresServerCheck(cityPath, cfg).Run(&CheckContext{CityPath: cityPath})
		want := "start PG (e.g. systemctl --user start postgresql, sudo systemctl start postgresql, or docker compose up -d postgres) then re-run gc doctor; or check the cloud provider's console / your VPN if this is a remote host"
		if result.FixHint != want {
			t.Fatalf("fix hint = %q, want %q", result.FixHint, want)
		}
	})

	t.Run("MultiScope_Mixed_AggregatesAsError", func(t *testing.T) {
		withPostgresServerDialResults(t, map[string]error{
			net.JoinHostPort("127.0.0.1", "5403"): errors.New("connection refused"),
		})
		withSystemdUserLinger(t, "linux", true, true, nil)
		cityPath, cfg := newPostgresServerCity(t,
			postgresServerScopeSpec{kind: "city", host: "127.0.0.1", port: "5401"},
			postgresServerScopeSpec{kind: "rig", name: "a", path: "rigs/a", host: "127.0.0.1", port: "5402"},
			postgresServerScopeSpec{kind: "rig", name: "b", path: "rigs/b", host: "127.0.0.1", port: "5403"},
		)

		result := NewPostgresServerCheck(cityPath, cfg).Run(&CheckContext{CityPath: cityPath, Verbose: true})
		if result.Status != StatusError {
			t.Fatalf("status = %v, want Error; result = %+v", result.Status, result)
		}
		if got, want := result.Message, "3 postgres-backed scope(s); first issue: server not reachable at 127.0.0.1:5403"; got != want {
			t.Fatalf("message = %q, want %q", got, want)
		}
		wantDetails := []string{
			"✗ rigs/b (127.0.0.1:5403) — server not reachable at 127.0.0.1:5403",
			"✓ city (127.0.0.1:5401) — reachable at 127.0.0.1:5401",
			"✓ rigs/a (127.0.0.1:5402) — reachable at 127.0.0.1:5402",
		}
		if !reflect.DeepEqual(result.Details, wantDetails) {
			t.Fatalf("details = %#v, want %#v", result.Details, wantDetails)
		}
	})

	t.Run("MultiScope_AllReachable_AggregatesAsOK", func(t *testing.T) {
		withPostgresServerDialResults(t, nil)
		withSystemdUserLinger(t, "darwin", true, false, nil)
		cityPath, cfg := newPostgresServerCity(t,
			postgresServerScopeSpec{kind: "city", host: "db.example.com", port: "5432"},
			postgresServerScopeSpec{kind: "rig", name: "frontend", path: "rigs/frontend", host: "db.example.com", port: "5433"},
		)

		result := NewPostgresServerCheck(cityPath, cfg).Run(&CheckContext{CityPath: cityPath, Verbose: true})
		if result.Status != StatusOK {
			t.Fatalf("status = %v, want OK; result = %+v", result.Status, result)
		}
		if got, want := result.Message, "2 postgres-backed scope(s) reachable"; got != want {
			t.Fatalf("message = %q, want %q", got, want)
		}
	})

	t.Run("Scope_MetadataEmpty_RaisesError", func(t *testing.T) {
		withPostgresServerDialResults(t, nil)
		withSystemdUserLinger(t, "linux", true, true, nil)
		cityPath, cfg := newPostgresServerCity(t, postgresServerScopeSpec{
			kind: "rig",
			name: "frontend",
			path: "rigs/frontend",
			host: "",
			port: "5432",
		})

		result := NewPostgresServerCheck(cityPath, cfg).Run(&CheckContext{CityPath: cityPath, Verbose: true})
		if result.Status != StatusError {
			t.Fatalf("status = %v, want Error; result = %+v", result.Status, result)
		}
		if got, want := result.Message, "metadata missing postgres host/port; cannot probe"; got != want {
			t.Fatalf("message = %q, want %q", got, want)
		}
		wantDetails := []string{"✗ rigs/frontend (:5432) — metadata missing postgres host/port; cannot probe"}
		if !reflect.DeepEqual(result.Details, wantDetails) {
			t.Fatalf("details = %#v, want %#v", result.Details, wantDetails)
		}
	})

	t.Run("Aggregation_DetailsSortedSeverityThenScope", func(t *testing.T) {
		withPostgresServerDialResults(t, map[string]error{
			net.JoinHostPort("127.0.0.1", "5402"): errors.New("connection refused"),
		})
		withSystemdUserLinger(t, "linux", true, false, nil)
		cityPath, cfg := newPostgresServerCity(t,
			postgresServerScopeSpec{kind: "city", host: "127.0.0.1", port: "5401"},
			postgresServerScopeSpec{kind: "rig", name: "a", path: "rigs/a", host: "127.0.0.1", port: "5402"},
			postgresServerScopeSpec{kind: "rig", name: "b", path: "rigs/b", host: "127.0.0.1", port: "5403"},
		)

		result := NewPostgresServerCheck(cityPath, cfg).Run(&CheckContext{CityPath: cityPath, Verbose: true})
		wantDetails := []string{
			"✗ rigs/a (127.0.0.1:5402) — server not reachable at 127.0.0.1:5402",
			"⚠ systemd-user linger is not enabled — PG will not start at boot",
			"✓ city (127.0.0.1:5401) — reachable at 127.0.0.1:5401",
			"✓ rigs/b (127.0.0.1:5403) — reachable at 127.0.0.1:5403",
		}
		if !reflect.DeepEqual(result.Details, wantDetails) {
			t.Fatalf("details = %#v, want %#v", result.Details, wantDetails)
		}
	})
}

func TestPostgresServerCheck_Linger(t *testing.T) {
	t.Run("LingerEnabled_StaysOK", func(t *testing.T) {
		withPostgresServerDialResults(t, nil)
		withSystemdUserLinger(t, "linux", true, true, nil)
		cityPath, cfg := newPostgresServerCity(t, postgresServerScopeSpec{kind: "city", host: "127.0.0.1", port: "5432"})

		result := NewPostgresServerCheck(cityPath, cfg).Run(&CheckContext{CityPath: cityPath, Verbose: true})
		if result.Status != StatusOK {
			t.Fatalf("status = %v, want OK; result = %+v", result.Status, result)
		}
		assertNoPostgresLingerRow(t, result.Details)
	})

	t.Run("LingerDisabled_WarnsOnLinuxLoopback", func(t *testing.T) {
		withPostgresServerDialResults(t, nil)
		withSystemdUserLinger(t, "linux", true, false, nil)
		cityPath, cfg := newPostgresServerCity(t, postgresServerScopeSpec{kind: "city", host: "127.0.0.1", port: "5432"})

		result := NewPostgresServerCheck(cityPath, cfg).Run(&CheckContext{CityPath: cityPath, Verbose: true})
		if result.Status != StatusWarning {
			t.Fatalf("status = %v, want Warning; result = %+v", result.Status, result)
		}
		if got, want := result.Message, "reachable at 127.0.0.1:5432; boot-survival is not configured"; got != want {
			t.Fatalf("message = %q, want %q", got, want)
		}
		assertPostgresLingerRow(t, result.Details)
		if result.FixHint != postgresServerLingerAmendment {
			t.Fatalf("fix hint = %q, want %q", result.FixHint, postgresServerLingerAmendment)
		}
	})

	t.Run("LingerDisabled_NotLinux_NoWarning", func(t *testing.T) {
		withPostgresServerDialResults(t, nil)
		withSystemdUserLinger(t, "darwin", true, false, nil)
		cityPath, cfg := newPostgresServerCity(t, postgresServerScopeSpec{kind: "city", host: "127.0.0.1", port: "5432"})

		result := NewPostgresServerCheck(cityPath, cfg).Run(&CheckContext{CityPath: cityPath, Verbose: true})
		if result.Status != StatusOK {
			t.Fatalf("status = %v, want OK; result = %+v", result.Status, result)
		}
		assertNoPostgresLingerRow(t, result.Details)
	})

	t.Run("LingerDisabled_NoSystemd_NoWarning", func(t *testing.T) {
		withPostgresServerDialResults(t, nil)
		withSystemdUserLinger(t, "linux", false, false, nil)
		cityPath, cfg := newPostgresServerCity(t, postgresServerScopeSpec{kind: "city", host: "127.0.0.1", port: "5432"})

		result := NewPostgresServerCheck(cityPath, cfg).Run(&CheckContext{CityPath: cityPath, Verbose: true})
		if result.Status != StatusOK {
			t.Fatalf("status = %v, want OK; result = %+v", result.Status, result)
		}
		assertNoPostgresLingerRow(t, result.Details)
	})

	t.Run("LingerDisabled_RemoteHost_NoWarning", func(t *testing.T) {
		withPostgresServerDialResults(t, nil)
		withSystemdUserLinger(t, "linux", true, false, nil)
		cityPath, cfg := newPostgresServerCity(t, postgresServerScopeSpec{kind: "city", host: "db.example.com", port: "5432"})

		result := NewPostgresServerCheck(cityPath, cfg).Run(&CheckContext{CityPath: cityPath, Verbose: true})
		if result.Status != StatusOK {
			t.Fatalf("status = %v, want OK; result = %+v", result.Status, result)
		}
		assertNoPostgresLingerRow(t, result.Details)
	})

	t.Run("UnreachableAndLingerDisabled_FixHintHasBoth", func(t *testing.T) {
		withPostgresServerDialResults(t, map[string]error{
			net.JoinHostPort("127.0.0.1", "5432"): errors.New("connection refused"),
		})
		withSystemdUserLinger(t, "linux", true, false, nil)
		cityPath, cfg := newPostgresServerCity(t, postgresServerScopeSpec{kind: "city", host: "127.0.0.1", port: "5432"})

		result := NewPostgresServerCheck(cityPath, cfg).Run(&CheckContext{CityPath: cityPath, Verbose: true})
		if result.Status != StatusError {
			t.Fatalf("status = %v, want Error; result = %+v", result.Status, result)
		}
		want := postgresServerLingerAmendment + " ; start PG (e.g. systemctl --user start postgresql, sudo systemctl start postgresql, or docker compose up -d postgres) then re-run gc doctor"
		if result.FixHint != want {
			t.Fatalf("fix hint = %q, want %q", result.FixHint, want)
		}
	})

	t.Run("LingerProbe_UserCurrentFails_DegradesGracefully", func(t *testing.T) {
		withPostgresServerDialResults(t, nil)
		withSystemdUserLinger(t, "linux", true, false, errors.New("whoami failed"))
		cityPath, cfg := newPostgresServerCity(t, postgresServerScopeSpec{kind: "city", host: "127.0.0.1", port: "5432"})

		result := NewPostgresServerCheck(cityPath, cfg).Run(&CheckContext{CityPath: cityPath, Verbose: true})
		if result.Status != StatusOK {
			t.Fatalf("status = %v, want OK; result = %+v", result.Status, result)
		}
		assertNoPostgresLingerRow(t, result.Details)
		want := "linger probe failed: whoami failed; PG boot-survival not verified"
		if !containsString(result.Details, want) {
			t.Fatalf("details = %#v, want %q", result.Details, want)
		}
	})
}

func TestPostgresServerFixHint(t *testing.T) {
	base := map[string]string{
		"linux":   "start PG (e.g. systemctl --user start postgresql, sudo systemctl start postgresql, or docker compose up -d postgres) then re-run gc doctor",
		"darwin":  "start PG (e.g. brew services start postgresql@<version>, launch Postgres.app, or docker compose up -d postgres) then re-run gc doctor",
		"windows": "start the PostgreSQL service (services.msc → PostgreSQL → Start, or pg_ctl start -D <data-dir>) then re-run gc doctor",
		"plan9":   "gc does not manage external PostgreSQL servers; start it via your OS supervisor or container runtime, then re-run gc doctor",
	}
	for _, goos := range []string{"linux", "darwin", "windows", "plan9"} {
		for _, lingerNeeded := range []bool{false, true} {
			for _, host := range []string{"127.0.0.1", "db.example.com"} {
				for _, port := range []string{"5432", "6543"} {
					t.Run(fmt.Sprintf("%s_linger_%t_host_%s_port_%s", goos, lingerNeeded, host, port), func(t *testing.T) {
						want := base[goos]
						if host == "db.example.com" {
							want += "; or check the cloud provider's console / your VPN if this is a remote host"
						}
						if lingerNeeded {
							want = postgresServerLingerAmendment + " ; " + want
						}
						if got := postgresServerFixHint(host, port, goos, lingerNeeded); got != want {
							t.Fatalf("postgresServerFixHint() = %q, want %q", got, want)
						}
					})
				}
			}
		}
	}
}

func TestSystemdUserLingerEnabled(t *testing.T) {
	t.Run("NonLinux_ReturnsFalseNil", func(t *testing.T) {
		withSystemdUserLingerSeams(t,
			func() string { return "darwin" },
			func(string) (os.FileInfo, error) {
				t.Fatal("stat probe should not run on non-Linux")
				return nil, nil
			},
			func() (*user.User, error) {
				t.Fatal("current user probe should not run on non-Linux")
				return nil, nil
			},
		)

		got, err := systemdUserLingerEnabled()
		if err != nil || got {
			t.Fatalf("systemdUserLingerEnabled() = (%t, %v), want (false, nil)", got, err)
		}
	})

	t.Run("Linux_NoSystemd_ReturnsFalseNil", func(t *testing.T) {
		withSystemdUserLingerSeams(t,
			func() string { return "linux" },
			func(path string) (os.FileInfo, error) {
				if path == systemdRuntimeDir {
					return nil, fs.ErrNotExist
				}
				t.Fatalf("unexpected stat path %q", path)
				return nil, nil
			},
			func() (*user.User, error) {
				t.Fatal("current user probe should not run when systemd is absent")
				return nil, nil
			},
		)

		got, err := systemdUserLingerEnabled()
		if err != nil || got {
			t.Fatalf("systemdUserLingerEnabled() = (%t, %v), want (false, nil)", got, err)
		}
	})

	t.Run("Linux_LingerFileExists_ReturnsTrueNil", func(t *testing.T) {
		withSystemdUserLinger(t, "linux", true, true, nil)

		got, err := systemdUserLingerEnabled()
		if err != nil || !got {
			t.Fatalf("systemdUserLingerEnabled() = (%t, %v), want (true, nil)", got, err)
		}
	})

	t.Run("Linux_LingerDirExistsButNoUserFile_ReturnsFalseNil", func(t *testing.T) {
		withSystemdUserLinger(t, "linux", true, false, nil)

		got, err := systemdUserLingerEnabled()
		if err != nil || got {
			t.Fatalf("systemdUserLingerEnabled() = (%t, %v), want (false, nil)", got, err)
		}
	})

	t.Run("Linux_StatPermissionError_Propagates", func(t *testing.T) {
		errPerm := errors.New("permission denied")
		withSystemdUserLingerSeams(t,
			func() string { return "linux" },
			func(path string) (os.FileInfo, error) {
				if path == systemdRuntimeDir {
					return fakePostgresServerFileInfo{}, nil
				}
				return nil, errPerm
			},
			func() (*user.User, error) {
				return &user.User{Username: "alice"}, nil
			},
		)

		got, err := systemdUserLingerEnabled()
		if !errors.Is(err, errPerm) || got {
			t.Fatalf("systemdUserLingerEnabled() = (%t, %v), want (false, %v)", got, err, errPerm)
		}
	})
}

type postgresServerScopeSpec struct {
	kind string
	name string
	path string
	host string
	port string
}

func newPostgresServerCity(t *testing.T, specs ...postgresServerScopeSpec) (string, *config.City) {
	t.Helper()
	cityPath := t.TempDir()
	cfg := &config.City{Workspace: config.Workspace{Name: "demo"}}
	for _, spec := range specs {
		switch spec.kind {
		case "city":
			writePostgresServerMetadata(t, cityPath, spec.host, spec.port)
		case "rig":
			if spec.path == "" {
				spec.path = filepath.Join("rigs", spec.name)
			}
			root := filepath.Join(cityPath, spec.path)
			writePostgresServerMetadata(t, root, spec.host, spec.port)
			cfg.Rigs = append(cfg.Rigs, config.Rig{
				Name: spec.name,
				Path: spec.path,
			})
		default:
			t.Fatalf("unknown scope kind %q", spec.kind)
		}
	}
	return cityPath, cfg
}

func writePostgresServerMetadata(t *testing.T, scopeRoot, host, port string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(scopeRoot, ".beads"), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(contract.MetadataState{
		Database:         "postgres",
		Backend:          "postgres",
		PostgresHost:     host,
		PostgresPort:     port,
		PostgresUser:     "bd",
		PostgresDatabase: "beads_pg",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scopeRoot, ".beads", "metadata.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func listenPostgresServer(t *testing.T) (string, string) {
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

func withPostgresServerDialResults(t *testing.T, failures map[string]error) {
	t.Helper()
	old := postgresServerDialContext
	postgresServerDialContext = func(_ context.Context, _, address string) (net.Conn, error) {
		if err, ok := failures[address]; ok {
			return nil, err
		}
		return noopPostgresServerConn{}, nil
	}
	t.Cleanup(func() { postgresServerDialContext = old })
}

type noopPostgresServerConn struct{}

func (noopPostgresServerConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (noopPostgresServerConn) Write(p []byte) (int, error)      { return len(p), nil }
func (noopPostgresServerConn) Close() error                     { return nil }
func (noopPostgresServerConn) LocalAddr() net.Addr              { return noopPostgresServerAddr("local") }
func (noopPostgresServerConn) RemoteAddr() net.Addr             { return noopPostgresServerAddr("remote") }
func (noopPostgresServerConn) SetDeadline(time.Time) error      { return nil }
func (noopPostgresServerConn) SetReadDeadline(time.Time) error  { return nil }
func (noopPostgresServerConn) SetWriteDeadline(time.Time) error { return nil }

type noopPostgresServerAddr string

func (a noopPostgresServerAddr) Network() string { return "tcp" }
func (a noopPostgresServerAddr) String() string  { return string(a) }

func withSystemdUserLinger(t *testing.T, goos string, systemdPresent, lingerExists bool, currentErr error) {
	t.Helper()
	withSystemdUserLingerSeams(t,
		func() string { return goos },
		func(path string) (os.FileInfo, error) {
			switch path {
			case systemdRuntimeDir:
				if systemdPresent {
					return fakePostgresServerFileInfo{}, nil
				}
				return nil, fs.ErrNotExist
			case filepath.Join(systemdLingerDir, "alice"):
				if lingerExists {
					return fakePostgresServerFileInfo{}, nil
				}
				return nil, fs.ErrNotExist
			default:
				return nil, fs.ErrNotExist
			}
		},
		func() (*user.User, error) {
			if currentErr != nil {
				return nil, currentErr
			}
			return &user.User{Username: "alice"}, nil
		},
	)
}

func withSystemdUserLingerSeams(t *testing.T, goos func() string, stat func(string) (os.FileInfo, error), current func() (*user.User, error)) {
	t.Helper()
	oldGOOS := systemdUserLingerGOOS
	oldStat := systemdUserLingerStatProbe
	oldCurrent := systemdUserLingerCurrentUser
	systemdUserLingerGOOS = goos
	systemdUserLingerStatProbe = stat
	systemdUserLingerCurrentUser = current
	t.Cleanup(func() {
		systemdUserLingerGOOS = oldGOOS
		systemdUserLingerStatProbe = oldStat
		systemdUserLingerCurrentUser = oldCurrent
	})
}

type fakePostgresServerFileInfo struct{}

func (fakePostgresServerFileInfo) Name() string       { return "fake" }
func (fakePostgresServerFileInfo) Size() int64        { return 0 }
func (fakePostgresServerFileInfo) Mode() os.FileMode  { return 0o644 }
func (fakePostgresServerFileInfo) ModTime() time.Time { return time.Time{} }
func (fakePostgresServerFileInfo) IsDir() bool        { return false }
func (fakePostgresServerFileInfo) Sys() any           { return nil }

func assertPostgresLingerRow(t *testing.T, details []string) {
	t.Helper()
	if !containsString(details, postgresServerLingerDetail) {
		t.Fatalf("details = %#v, want linger row %q", details, postgresServerLingerDetail)
	}
}

func assertNoPostgresLingerRow(t *testing.T, details []string) {
	t.Helper()
	if containsString(details, postgresServerLingerDetail) {
		t.Fatalf("details = %#v, want no linger row", details)
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
