package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lookIn finds the named tools under /usr/bin. On Windows a test that finds one
// is skipped: the backends wrap unix processes, and /usr/bin/x is not an
// absolute path there.
func lookIn(t *testing.T, tools ...string) func(string) (string, error) {
	t.Helper()
	return func(name string) (string, error) {
		for _, tool := range tools {
			if tool == name {
				if runtime.GOOS == "windows" {
					t.Skip("the sandbox backends confine unix processes; their fake paths are not absolute on windows")
				}
				return "/usr/bin/" + name, nil
			}
		}
		return "", exec.ErrNotFound
	}
}

func TestParseMode(t *testing.T) {
	tests := []struct {
		in      string
		want    Mode
		wantErr bool
	}{
		{"", ModeAuto, false},
		{"auto", ModeAuto, false},
		{" Require ", ModeRequire, false},
		{"none", ModeNone, false},
		{"yes", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseMode(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWrapBuildsTheBackendCommand(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	tests := []struct {
		name        string
		goos        string
		tools       []string
		spec        Spec
		backend     Backend
		wantPrefix  []string
		wantContain []string
		noWrites    bool
	}{
		{"darwin", "darwin", []string{"sandbox-exec"}, Spec{WriteDirs: []string{dir}}, BackendSandboxExec,
			[]string{"/usr/bin/sandbox-exec", "-D", "W0=" + real, "-p"}, []string{"(deny network*)", "(param \"W0\")"}, true},
		{"darwin with network", "darwin", []string{"sandbox-exec"}, Spec{WriteDirs: []string{dir}, AllowNetwork: true}, BackendSandboxExec,
			[]string{"/usr/bin/sandbox-exec"}, []string{"(deny file-write*)"}, true},
		{"linux bwrap", "linux", []string{"bwrap", "unshare"}, Spec{WriteDirs: []string{dir}}, BackendBwrap,
			[]string{"/usr/bin/bwrap", "--die-with-parent", "--new-session", "--unshare-pid", "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "--unshare-net", "--bind", real, real, "--"}, nil, true},
		{"linux unshare", "linux", []string{"unshare"}, Spec{}, BackendUnshare,
			[]string{"/usr/bin/unshare", "--net", "--map-root-user", "--"}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			sb := New(tt.goos, lookIn(t, tt.tools...))
			// Act
			w, err := sb.Wrap(tt.spec, []string{"/bin/echo", "hi"})
			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.backend, w.Backend)
			assert.Equal(t, tt.noWrites, w.NoWrites)
			assert.Equal(t, tt.backend, sb.Backend())
			assert.Equal(t, tt.wantPrefix, w.Argv[:len(tt.wantPrefix)])
			assert.Equal(t, []string{"/bin/echo", "hi"}, w.Argv[len(w.Argv)-2:])
			joined := strings.Join(w.Argv, "\n")
			for _, c := range tt.wantContain {
				assert.Contains(t, joined, c)
			}
		})
	}
}

func TestWrapKeepsPathsOutOfTheProfileText(t *testing.T) {
	// A write directory with quote and paren characters must not reach the profile.
	if runtime.GOOS == "windows" {
		t.Skip("windows file names cannot hold the quote characters this test needs; sandbox-exec is macOS only")
	}
	dir := filepath.Join(t.TempDir(), `a")(allow network*) ("`)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	w, err := New("darwin", lookIn(t, "sandbox-exec")).Wrap(Spec{WriteDirs: []string{dir}}, []string{"/bin/true"})
	require.NoError(t, err)
	profile := w.Argv[slicesIndex(w.Argv, "-p")+1]
	assert.NotContains(t, profile, "allow network")
	assert.NotContains(t, profile, filepath.Base(dir))
}

func slicesIndex(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func TestUnavailableRefuses(t *testing.T) {
	tests := []struct {
		name  string
		goos  string
		tools []string
	}{
		{"windows", "windows", []string{"bwrap", "sandbox-exec"}},
		{"freebsd", "freebsd", []string{"unshare"}},
		{"linux without tools", "linux", nil},
		{"darwin without sandbox-exec", "darwin", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := New(tt.goos, lookIn(t, tt.tools...))
			_, err := sb.Wrap(Spec{}, []string{"x"})
			require.ErrorIs(t, err, ErrUnavailable)
			assert.Equal(t, BackendNone, sb.Backend())
			// auto runs unconfined, require refuses, none never asks.
			on, err := sb.Resolve(context.Background(), ModeAuto)
			require.NoError(t, err)
			assert.False(t, on)
			_, err = sb.Resolve(context.Background(), ModeRequire)
			require.ErrorIs(t, err, ErrUnavailable)
			on, err = sb.Resolve(context.Background(), ModeNone)
			require.NoError(t, err)
			assert.False(t, on)
		})
	}
}

func TestWrapRejectsAnEmptyCommand(t *testing.T) {
	_, err := New("darwin", lookIn(t, "sandbox-exec")).Wrap(Spec{}, nil)
	require.Error(t, err)
}

// The tests below run a real confined process. They skip where the platform has
// no usable backend (CI containers without user namespaces).

const helperEnv = "AI_RULEZ_SANDBOX_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) != "" {
		os.Exit(helper())
	}
	os.Exit(m.Run())
}

// requireSandboxEnv makes a missing backend a failure instead of a skip, for a
// run that must prove confinement (the unprivileged Linux container stage).
const requireSandboxEnv = "AI_RULEZ_REQUIRE_SANDBOX"

func realSandbox(t *testing.T) *Sandbox {
	t.Helper()
	sb := Default()
	if err := sb.Check(context.Background()); err != nil {
		if os.Getenv(requireSandboxEnv) != "" {
			t.Fatalf("%s is set but no process isolation works: %v", requireSandboxEnv, err)
		}
		t.Skipf("no usable process isolation: %v", err)
	}
	return sb
}

// runHelper runs this test binary as the helper under the sandbox and returns its output.
func runHelper(t *testing.T, sb *Sandbox, spec Spec, mode string, arg string) (string, error) {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	w, err := sb.Wrap(spec, []string{exe, mode, arg})
	require.NoError(t, err)
	cmd := exec.Command(w.Argv[0], w.Argv[1:]...) //nolint:gosec // test helper
	cmd.Env = append(os.Environ(), helperEnv+"=1")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func TestConfinedProcessCannotUseTheNetwork(t *testing.T) {
	sb := realSandbox(t)
	dir := t.TempDir()
	addr := listenHere(t)
	out, err := runHelper(t, sb, Spec{WriteDirs: []string{dir}}, "dial", addr)
	require.NoError(t, err, out)
	assert.Equal(t, "denied", out)
	// Control: the same helper outside the sandbox can connect.
	exe, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.Command(exe, "dial", addr)
	cmd.Env = append(os.Environ(), helperEnv+"=1")
	plain, err := cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, "allowed", strings.TrimSpace(string(plain)))
}

func TestConfinedProcessWritesOnlyInsideTheNamedDirs(t *testing.T) {
	sb := realSandbox(t)
	if sb.Backend() == BackendUnshare {
		t.Skip("unshare confines the network only")
	}
	inside, outside := t.TempDir(), t.TempDir()
	out, err := runHelper(t, sb, Spec{WriteDirs: []string{inside}}, "write", filepath.Join(inside, "ok.txt"))
	require.NoError(t, err, out)
	assert.Equal(t, "written", out)
	assert.FileExists(t, filepath.Join(inside, "ok.txt"))

	out, err = runHelper(t, sb, Spec{WriteDirs: []string{inside}}, "write", filepath.Join(outside, "no.txt"))
	require.NoError(t, err, out)
	assert.Equal(t, "denied", out)
	assert.NoFileExists(t, filepath.Join(outside, "no.txt"))
}

func TestConfinedProcessKeepsReadAccess(t *testing.T) {
	sb := realSandbox(t)
	file := filepath.Join(t.TempDir(), "in.txt")
	require.NoError(t, os.WriteFile(file, []byte("data"), 0o600))
	out, err := runHelper(t, sb, Spec{WriteDirs: []string{t.TempDir()}}, "read", file)
	require.NoError(t, err, out)
	assert.Equal(t, "data", out)
}

func TestAllowNetworkLiftsOnlyTheNetworkDenial(t *testing.T) {
	sb := realSandbox(t)
	if sb.Backend() == BackendUnshare {
		t.Skip("unshare without --net is a no-op")
	}
	inside, outside := t.TempDir(), t.TempDir()
	out, err := runHelper(t, sb, Spec{WriteDirs: []string{inside}, AllowNetwork: true}, "dial", listenHere(t))
	require.NoError(t, err, out)
	assert.Equal(t, "allowed", out)
	out, err = runHelper(t, sb, Spec{WriteDirs: []string{inside}, AllowNetwork: true}, "write", filepath.Join(outside, "no.txt"))
	require.NoError(t, err, out)
	assert.Equal(t, "denied", out)
}

func TestDarwinUsesSandboxExec(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	realSandbox(t)
	assert.Equal(t, BackendSandboxExec, Default().Backend())
}

// helper is the confined side: it prints one word describing what it could do.
func helper() int {
	args := os.Args[1:]
	if len(args) < 1 {
		return 2
	}
	switch args[0] {
	case "dial":
		fmt.Println(netProbe(args[1]))
	case "write":
		if err := os.WriteFile(args[1], []byte("x"), 0o600); err != nil {
			fmt.Println("denied")
		} else {
			fmt.Println("written")
		}
	case "read":
		data, err := os.ReadFile(args[1])
		if err != nil {
			fmt.Println("error:", err)
			return 1
		}
		fmt.Print(string(data))
	case "inject":
		fmt.Println(injectProbe(args[1]))
	}
	return 0
}
