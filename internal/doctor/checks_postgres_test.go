package doctor

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPostgresServerFixHint_NonSystemdLinuxNoService_PointsAtNonSystemdDoc(t *testing.T) {
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos: "linux",
	})

	got, ok := postgresServerBootstrapFixHint([]postgresServerScope{
		{host: "127.0.0.1", port: "5433"},
	}, "linux")
	if !ok {
		t.Fatal("postgresServerBootstrapFixHint() ok = false, want true")
	}
	if got != postgresServerNonSystemdBootstrapAmendment {
		t.Fatalf("hint = %q, want %q", got, postgresServerNonSystemdBootstrapAmendment)
	}
}

func TestPostgresServerFixHint_NonSystemdLinuxServicePresent_KeepsBaseHint(t *testing.T) {
	for _, tc := range []struct {
		name string
		path func(home string) string
	}{
		{
			name: "OpenRC",
			path: func(string) string {
				return beadsPostgresOpenRCServiceFile
			},
		},
		{
			name: "runit",
			path: func(home string) string {
				return filepath.Join(home, beadsPostgresRunitServiceFile)
			},
		},
		{
			name: "s6",
			path: func(home string) string {
				return filepath.Join(home, beadsPostgresS6ServiceFile)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := "/home/alice"
			withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
				goos:         "linux",
				home:         home,
				presentPaths: []string{tc.path(home)},
			})

			got, ok := postgresServerBootstrapFixHint([]postgresServerScope{
				{host: "127.0.0.1", port: "5433"},
			}, "linux")
			if ok {
				t.Fatalf("postgresServerBootstrapFixHint() = (%q, true), want fall-through", got)
			}
		})
	}
}

func TestPostgresServerFixHint_SystemdLinuxTakesPrecedenceOverNonSystemd(t *testing.T) {
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos:           "linux",
		systemdPresent: true,
	})

	got, ok := postgresServerBootstrapFixHint([]postgresServerScope{
		{host: "127.0.0.1", port: "5433"},
	}, "linux")
	if !ok {
		t.Fatal("postgresServerBootstrapFixHint() ok = false, want true")
	}
	if got != postgresServerBootstrapAmendment {
		t.Fatalf("hint = %q, want systemd hint %q", got, postgresServerBootstrapAmendment)
	}
}

func TestPostgresServerFixHint_NonSystemdLinux_MixedRemote_AppendsCloudHint(t *testing.T) {
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos: "linux",
	})

	got, ok := postgresServerBootstrapFixHint([]postgresServerScope{
		{host: "127.0.0.1", port: "5433"},
		{host: "db.example.com", port: "5432"},
	}, "linux")
	if !ok {
		t.Fatal("postgresServerBootstrapFixHint() ok = false, want true")
	}
	want := postgresServerNonSystemdBootstrapAmendment + "; or check the cloud provider's console / your VPN if this is a remote host"
	if got != want {
		t.Fatalf("hint = %q, want %q", got, want)
	}
}

func TestPostgresServerFixHint_NonSystemdLinux_OK_NoAmendment(t *testing.T) {
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos: "linux",
	})

	got, ok := postgresServerBootstrapFixHint(nil, "linux")
	if ok || got != "" {
		t.Fatalf("postgresServerBootstrapFixHint() = (%q, %t), want empty false", got, ok)
	}
}

func TestPostgresServerFixHint_ContainerNotRunning_LoopbackFailing_PointsAtContainerDoc(t *testing.T) {
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos:         "linux",
		presentPaths: []string{beadsPostgresOpenRCServiceFile},
	})
	withPostgresContainerInspectExec(t, func(context.Context, string, ...string) ([]byte, error) {
		return []byte("false\n"), nil
	})

	got, ok := postgresServerBootstrapFixHint([]postgresServerScope{
		{host: "127.0.0.1", port: "5433"},
	}, "linux")
	if !ok {
		t.Fatal("postgresServerBootstrapFixHint() ok = false, want true")
	}
	if got != postgresServerContainerBootstrapAmendment {
		t.Fatalf("hint = %q, want %q", got, postgresServerContainerBootstrapAmendment)
	}
}

func TestPostgresServerFixHint_ContainerRunning_LoopbackFailing_NoContainerAmendment(t *testing.T) {
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos:         "linux",
		presentPaths: []string{beadsPostgresOpenRCServiceFile},
	})
	withPostgresContainerInspectExec(t, func(context.Context, string, ...string) ([]byte, error) {
		return []byte("true\n"), nil
	})

	got, ok := postgresServerBootstrapFixHint([]postgresServerScope{
		{host: "127.0.0.1", port: "5433"},
	}, "linux")
	if ok || got != "" {
		t.Fatalf("postgresServerBootstrapFixHint() = (%q, %t), want empty false", got, ok)
	}
}

func TestPostgresServerFixHint_ContainerNotRunning_MixedRemote_AppendsCloudHint(t *testing.T) {
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos:         "linux",
		presentPaths: []string{beadsPostgresOpenRCServiceFile},
	})
	withPostgresContainerInspectExec(t, func(context.Context, string, ...string) ([]byte, error) {
		return []byte("false\n"), nil
	})

	got, ok := postgresServerBootstrapFixHint([]postgresServerScope{
		{host: "127.0.0.1", port: "5433"},
		{host: "db.example.com", port: "5432"},
	}, "linux")
	if !ok {
		t.Fatal("postgresServerBootstrapFixHint() ok = false, want true")
	}
	want := postgresServerContainerBootstrapAmendment + postgresServerRemoteHostHintSuffix
	if got != want {
		t.Fatalf("hint = %q, want %q", got, want)
	}
}

func TestPostgresServerFixHint_ContainerNotRunning_NoLoopbackFailing_NoAmendment(t *testing.T) {
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos: "linux",
	})
	withPostgresContainerInspectExec(t, func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("container runtime probe should not run without loopback failures")
		return nil, nil
	})

	got, ok := postgresServerBootstrapFixHint([]postgresServerScope{
		{host: "db.example.com", port: "5432"},
	}, "linux")
	if ok || got != "" {
		t.Fatalf("postgresServerBootstrapFixHint() = (%q, %t), want empty false", got, ok)
	}
}

func TestPostgresServerFixHint_LinuxUnitMissing_ContainerAmendmentSuppressed(t *testing.T) {
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos:           "linux",
		systemdPresent: true,
	})
	withPostgresContainerInspectExec(t, func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("container runtime probe should not run when systemd bootstrap amendment fires")
		return nil, nil
	})

	got, ok := postgresServerBootstrapFixHint([]postgresServerScope{
		{host: "127.0.0.1", port: "5433"},
	}, "linux")
	if !ok {
		t.Fatal("postgresServerBootstrapFixHint() ok = false, want true")
	}
	if got != postgresServerBootstrapAmendment {
		t.Fatalf("hint = %q, want %q", got, postgresServerBootstrapAmendment)
	}
}

func TestPostgresServerFixHint_ContainerTimeout_DegradesToBaseHint(t *testing.T) {
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos:         "linux",
		presentPaths: []string{beadsPostgresOpenRCServiceFile},
	})
	withPostgresContainerInspectExec(t, func(context.Context, string, ...string) ([]byte, error) {
		return nil, context.DeadlineExceeded
	})

	got, ok := postgresServerBootstrapFixHint([]postgresServerScope{
		{host: "127.0.0.1", port: "5433"},
	}, "linux")
	if ok || got != "" {
		t.Fatalf("postgresServerBootstrapFixHint() = (%q, %t), want empty false", got, ok)
	}
}

func TestPostgresServerFixHint_NonSystemdLinux_UserCurrentFails_DegradesToBaseHint(t *testing.T) {
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos:       "linux",
		currentErr: errors.New("whoami failed"),
	})

	got, ok := postgresServerBootstrapFixHint([]postgresServerScope{
		{host: "127.0.0.1", port: "5433"},
	}, "linux")
	if ok {
		t.Fatalf("postgresServerBootstrapFixHint() = (%q, true), want fall-through", got)
	}
}

func TestPostgresServerFixHint_NonSystemdLinux_CanFixFalse(t *testing.T) {
	if NewPostgresServerCheck(t.TempDir(), nil).CanFix() {
		t.Fatal("PostgresServerCheck.CanFix() = true, want false")
	}
}

func TestBeadsPostgresNonSystemdServicePaths_Defaults(t *testing.T) {
	assertPostgresNonSystemdPath(t, "OpenRC", beadsPostgresOpenRCServiceFile, "/etc/init.d/beads-postgres")
	assertPostgresNonSystemdPath(t, "runit", beadsPostgresRunitServiceFile, ".local/sv/beads-postgres/run")
	assertPostgresNonSystemdPath(t, "s6", beadsPostgresS6ServiceFile, ".s6/service/beads-postgres/run")
}

func TestBeadsPostgresNonSystemdServiceInstalled_OpenRC_True(t *testing.T) {
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos:         "linux",
		presentPaths: []string{beadsPostgresOpenRCServiceFile},
	})

	got, err := beadsPostgresNonSystemdServiceInstalled()
	if err != nil || !got {
		t.Fatalf("beadsPostgresNonSystemdServiceInstalled() = (%t, %v), want (true, nil)", got, err)
	}
}

func TestBeadsPostgresNonSystemdServiceInstalled_Runit_True(t *testing.T) {
	home := "/home/alice"
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos:         "linux",
		home:         home,
		presentPaths: []string{filepath.Join(home, beadsPostgresRunitServiceFile)},
	})

	got, err := beadsPostgresNonSystemdServiceInstalled()
	if err != nil || !got {
		t.Fatalf("beadsPostgresNonSystemdServiceInstalled() = (%t, %v), want (true, nil)", got, err)
	}
}

func TestBeadsPostgresNonSystemdServiceInstalled_S6_True(t *testing.T) {
	home := "/home/alice"
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos:         "linux",
		home:         home,
		presentPaths: []string{filepath.Join(home, beadsPostgresS6ServiceFile)},
	})

	got, err := beadsPostgresNonSystemdServiceInstalled()
	if err != nil || !got {
		t.Fatalf("beadsPostgresNonSystemdServiceInstalled() = (%t, %v), want (true, nil)", got, err)
	}
}

func TestBeadsPostgresNonSystemdServiceInstalled_None_False(t *testing.T) {
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos: "linux",
	})

	got, err := beadsPostgresNonSystemdServiceInstalled()
	if err != nil || got {
		t.Fatalf("beadsPostgresNonSystemdServiceInstalled() = (%t, %v), want (false, nil)", got, err)
	}
}

func TestBeadsPostgresNonSystemdServiceInstalled_NonLinux_FalseNoProbe(t *testing.T) {
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos:             "darwin",
		failUserIfCalled: true,
	})

	got, err := beadsPostgresNonSystemdServiceInstalled()
	if err != nil || got {
		t.Fatalf("beadsPostgresNonSystemdServiceInstalled() = (%t, %v), want (false, nil)", got, err)
	}
}

func TestBeadsPostgresNonSystemdServiceInstalled_SystemdLinux_FalseNoUserProbe(t *testing.T) {
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos:             "linux",
		systemdPresent:   true,
		failUserIfCalled: true,
	})

	got, err := beadsPostgresNonSystemdServiceInstalled()
	if err != nil || got {
		t.Fatalf("beadsPostgresNonSystemdServiceInstalled() = (%t, %v), want (false, nil)", got, err)
	}
}

func TestBeadsPostgresNonSystemdServiceInstalled_UserCurrentFails_DegradesGracefully(t *testing.T) {
	wantErr := errors.New("whoami failed")
	withPostgresNonSystemdSeams(t, postgresNonSystemdSeams{
		goos:       "linux",
		currentErr: wantErr,
	})

	got, err := beadsPostgresNonSystemdServiceInstalled()
	if !errors.Is(err, wantErr) || got {
		t.Fatalf("beadsPostgresNonSystemdServiceInstalled() = (%t, %v), want (false, %v)", got, err, wantErr)
	}
}

func TestBeadsPostgresContainerRunning_True(t *testing.T) {
	withPostgresContainerInspectExec(t, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		assertPostgresContainerInspectCall(ctx, t, name, args)
		return []byte("true\n"), nil
	})

	got, err := beadsPostgresContainerRunning()
	if err != nil || !got {
		t.Fatalf("beadsPostgresContainerRunning() = (%t, %v), want (true, nil)", got, err)
	}
}

func TestBeadsPostgresContainerRunning_FalseStopped(t *testing.T) {
	withPostgresContainerInspectExec(t, func(context.Context, string, ...string) ([]byte, error) {
		return []byte("false\n"), nil
	})

	got, err := beadsPostgresContainerRunning()
	if err != nil || got {
		t.Fatalf("beadsPostgresContainerRunning() = (%t, %v), want (false, nil)", got, err)
	}
}

func TestBeadsPostgresContainerRunning_FallbacksToPodmanWhenDockerUnavailable(t *testing.T) {
	var calls []string
	withPostgresContainerInspectExec(t, func(_ context.Context, name string, _ ...string) ([]byte, error) {
		calls = append(calls, name)
		if name == "docker" {
			return nil, exec.ErrNotFound
		}
		return []byte("true\n"), nil
	})

	got, err := beadsPostgresContainerRunning()
	if err != nil || !got {
		t.Fatalf("beadsPostgresContainerRunning() = (%t, %v), want (true, nil)", got, err)
	}
	if strings.Join(calls, ",") != "docker,podman" {
		t.Fatalf("runtime calls = %v, want docker then podman", calls)
	}
}

func TestBeadsPostgresContainerRunning_NoCLI_ReturnsNil(t *testing.T) {
	withPostgresContainerInspectExec(t, func(context.Context, string, ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	})

	got, err := beadsPostgresContainerRunning()
	if err != nil || got {
		t.Fatalf("beadsPostgresContainerRunning() = (%t, %v), want (false, nil)", got, err)
	}
}

func TestBeadsPostgresContainerRunning_NoContainer_ReturnsNil(t *testing.T) {
	withPostgresContainerInspectExec(t, func(context.Context, string, ...string) ([]byte, error) {
		return []byte("Error: No such object: beads-postgres\n"), errors.New("exit status 1")
	})

	got, err := beadsPostgresContainerRunning()
	if err != nil || got {
		t.Fatalf("beadsPostgresContainerRunning() = (%t, %v), want (false, nil)", got, err)
	}
}

func TestBeadsPostgresContainerRunning_UnexpectedError(t *testing.T) {
	wantErr := errors.New("permission denied")
	withPostgresContainerInspectExec(t, func(context.Context, string, ...string) ([]byte, error) {
		return []byte("permission denied\n"), wantErr
	})

	got, err := beadsPostgresContainerRunning()
	if !errors.Is(err, wantErr) || got {
		t.Fatalf("beadsPostgresContainerRunning() = (%t, %v), want (false, %v)", got, err, wantErr)
	}
}

func TestBeadsPostgresContainerRunning_Timeout(t *testing.T) {
	withPostgresContainerInspectExec(t, func(context.Context, string, ...string) ([]byte, error) {
		return nil, context.DeadlineExceeded
	})

	got, err := beadsPostgresContainerRunning()
	if !errors.Is(err, context.DeadlineExceeded) || got {
		t.Fatalf("beadsPostgresContainerRunning() = (%t, %v), want (false, deadline exceeded)", got, err)
	}
}

func assertPostgresNonSystemdPath(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s path = %q, want %q", name, got, want)
	}
}

func withPostgresContainerInspectExec(t *testing.T, execFunc func(context.Context, string, ...string) ([]byte, error)) {
	t.Helper()
	old := beadsPostgresContainerInspectExec
	beadsPostgresContainerInspectExec = execFunc
	t.Cleanup(func() { beadsPostgresContainerInspectExec = old })
}

func assertPostgresContainerInspectCall(ctx context.Context, t *testing.T, name string, args []string) {
	t.Helper()
	if name != "docker" {
		t.Fatalf("runtime = %q, want docker", name)
	}
	wantArgs := []string{"inspect", "--format", "{{.State.Running}}", beadsPostgresContainerName}
	if strings.Join(args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %q, want %q", args, wantArgs)
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("container inspect context has no deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > 5*time.Second {
		t.Fatalf("container inspect timeout remaining = %s, want within 5s", remaining)
	}
}

type postgresNonSystemdSeams struct {
	goos             string
	home             string
	systemdPresent   bool
	currentErr       error
	presentPaths     []string
	failUserIfCalled bool
}

func withPostgresNonSystemdSeams(t *testing.T, seams postgresNonSystemdSeams) {
	t.Helper()
	if seams.goos == "" {
		seams.goos = "linux"
	}
	if seams.home == "" {
		seams.home = "/home/alice"
	}
	present := make(map[string]bool, len(seams.presentPaths))
	for _, path := range seams.presentPaths {
		present[path] = true
	}

	oldGOOS := systemdUserLingerGOOS
	oldStat := systemdUserLingerStatProbe
	oldCurrent := systemdUserLingerCurrentUser
	systemdUserLingerGOOS = func() string { return seams.goos }
	systemdUserLingerStatProbe = func(path string) (os.FileInfo, error) {
		if path == systemdRuntimeDir {
			if seams.systemdPresent {
				return fakePostgresNonSystemdFileInfo{}, nil
			}
			return nil, fs.ErrNotExist
		}
		if present[path] {
			return fakePostgresNonSystemdFileInfo{}, nil
		}
		return nil, fs.ErrNotExist
	}
	systemdUserLingerCurrentUser = func() (*user.User, error) {
		if seams.failUserIfCalled {
			t.Fatal("current user probe should not run")
		}
		if seams.currentErr != nil {
			return nil, seams.currentErr
		}
		return &user.User{HomeDir: seams.home, Username: "alice"}, nil
	}
	t.Cleanup(func() {
		systemdUserLingerGOOS = oldGOOS
		systemdUserLingerStatProbe = oldStat
		systemdUserLingerCurrentUser = oldCurrent
	})
}

type fakePostgresNonSystemdFileInfo struct{}

func (fakePostgresNonSystemdFileInfo) Name() string       { return "fake" }
func (fakePostgresNonSystemdFileInfo) Size() int64        { return 0 }
func (fakePostgresNonSystemdFileInfo) Mode() os.FileMode  { return 0o644 }
func (fakePostgresNonSystemdFileInfo) ModTime() time.Time { return time.Time{} }
func (fakePostgresNonSystemdFileInfo) IsDir() bool        { return false }
func (fakePostgresNonSystemdFileInfo) Sys() any           { return nil }
