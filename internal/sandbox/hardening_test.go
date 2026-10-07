package sandbox

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

func TestBwrapDetachesTheSession(t *testing.T) {
	// Arrange
	sb := New("linux", lookIn(t, "bwrap"))
	// Act
	w, err := sb.Wrap(Spec{}, []string{"/bin/true"})
	// Assert
	require.NoError(t, err)
	for _, flag := range []string{"--die-with-parent", "--new-session", "--unshare-pid"} {
		assert.Contains(t, w.Argv, flag)
	}
}

func TestProfileDeniesLaunchServices(t *testing.T) {
	profile := sandboxProfile(Spec{}, 0)
	assert.Contains(t, profile, "(deny mach-lookup")
	for _, svc := range []string{"com.apple.coreservices.launchservicesd", "com.apple.coreservices.appleevents", "com.apple.lsd."} {
		assert.Contains(t, profile, svc)
	}
}

func TestConfinedProcessCannotLaunchApplications(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only: LaunchServices")
	}
	sb := realSandbox(t)
	if _, err := os.Stat("/usr/bin/open"); err != nil {
		t.Skip("no open(1)")
	}
	// Arrange
	w, err := sb.Wrap(Spec{WriteDirs: []string{t.TempDir()}}, []string{"/usr/bin/open", "-g", "-j", "-b", "com.apple.TextEdit"})
	require.NoError(t, err)
	// Act
	out, err := exec.Command(w.Argv[0], w.Argv[1:]...).CombinedOutput() //nolint:gosec // test argv
	// Assert: without the Mach denial the same command starts TextEdit.
	require.Error(t, err, "an application was launched from inside the sandbox: %s", out)
	assert.Contains(t, string(out), "LSCopyApplicationURLsForBundleIdentifier")
}

func TestCheckSurvivesACancelledContext(t *testing.T) {
	// Arrange: the caller's context is already canceled.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fake := &runner.Fake{}
	sb := New("linux", lookIn(t, "bwrap")).WithRunner(fake)
	// Act
	err := sb.Check(ctx)
	// Assert: the probe ran to completion and the answer is cached for later callers.
	require.NoError(t, err)
	require.NoError(t, sb.Check(context.Background()))
	require.Len(t, fake.Calls(), 1)
}

func TestCheckFallsBackToABackendThatWorks(t *testing.T) {
	// A container that permits user namespaces but not a new /proc mount: bwrap
	// is installed and fails its probe ("Can't mount proc on /proc"), unshare works.
	tests := []struct {
		name        string
		tools       []string
		failing     string
		wantBackend Backend
		wantErr     string
	}{
		{"bwrap fails, unshare works", []string{"bwrap", "unshare"}, "bwrap", BackendUnshare, ""},
		{"bwrap works", []string{"bwrap", "unshare"}, "", BackendBwrap, ""},
		{"both fail", []string{"bwrap", "unshare"}, "*", BackendNone, "Can't mount proc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			fake := &runner.Fake{Handle: func(s runner.Spec) runner.Result {
				if tt.failing == "*" || strings.HasSuffix(s.Argv[0], "/"+tt.failing) {
					return runner.Result{Status: runner.StatusExit, ExitCode: 1, Stderr: []byte("bwrap: Can't mount proc on /proc")}
				}
				return runner.Result{Status: runner.StatusOK}
			}}
			sb := New("linux", lookIn(t, tt.tools...)).WithRunner(fake)
			// Act
			err := sb.Check(context.Background())
			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantBackend, sb.Backend())
			w, err := sb.Wrap(Spec{}, []string{"/bin/true"})
			require.NoError(t, err)
			assert.Equal(t, tt.wantBackend, w.Backend)
		})
	}
}

func TestCheckReportsAProbeFailure(t *testing.T) {
	fake := &runner.Fake{Handle: func(runner.Spec) runner.Result {
		return runner.Result{Status: runner.StatusExit, ExitCode: 1, Stderr: []byte("bwrap: no permissions")}
	}}
	err := New("linux", lookIn(t, "bwrap")).WithRunner(fake).Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no permissions")
}
