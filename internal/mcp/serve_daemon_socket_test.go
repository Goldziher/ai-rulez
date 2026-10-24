//go:build !windows

package mcp

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// warnRecorder is a logger that keeps the messages logged at warn level.
type warnRecorder struct {
	mu    sync.Mutex
	warns []string
}

func (r *warnRecorder) Debug(string, ...any) {}
func (r *warnRecorder) Info(string, ...any)  {}
func (r *warnRecorder) Error(string, ...any) {}
func (r *warnRecorder) Warn(msg string, _ ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.warns = append(r.warns, msg)
}

func longHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), strings.Repeat("h", 80))
	require.NoError(t, os.MkdirAll(home, 0o700))
	t.Setenv("HOME", home)
	return home
}

// A home so long that the cache path cannot hold a socket moves the socket under
// the temp directory instead of dropping the daemon (#483).
func TestDaemonSocketPath_LongHomeFallsBackToTheTempDirectory(t *testing.T) {
	longHome(t)
	short, err := os.MkdirTemp("", "ard-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(short) })
	t.Setenv("XDG_RUNTIME_DIR", short)
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	st := &ServeSetup{Version: "5.0.3", WorkDir: root, DaemonIdentity: "one"}

	path, err := st.daemonSocketPath()

	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(path, short), "the socket is under the runtime directory: %s", path)
	assert.LessOrEqual(t, len(path), socketPathLimit())
	again, err := st.daemonSocketPath()
	require.NoError(t, err)
	assert.Equal(t, path, again, "the fallback path is stable, so sessions share one daemon")
}

// When even the temp directory is too long there is no socket; the session says
// so at warn level and serves in-process.
func TestServeViaDaemon_WarnsWhenNoSocketCanBePlaced(t *testing.T) {
	longHome(t)
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(t.TempDir(), strings.Repeat("t", 90)))
	rec := &warnRecorder{}
	root := t.TempDir()

	handled, err := ServeViaDaemon(context.Background(), &ServeSetup{WorkDir: root, Log: rec},
		bytes.NewReader(nil), io.Discard, ProxyOptions{})

	require.NoError(t, err)
	assert.False(t, handled)
	require.Len(t, rec.warns, 1)
	assert.Contains(t, rec.warns[0], "cannot place a socket")
}

// A directory under a shared temp directory that another user owns is refused.
func TestEnsureSocketDir_RefusesADirectoryOfAnotherOwner(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("needs a unix user other than root")
	}
	dir := filepath.Join(t.TempDir(), "d")
	require.NoError(t, ensureSocketDir(dir), "our own 0700 directory is accepted")
	require.NoError(t, os.Chmod(dir, 0o755))
	assert.Error(t, ensureSocketDir(dir), "a directory open to others is refused")
}

// The daemon leaves when the repository it serves is removed (#481), without
// waiting out the idle timeout.
func TestDaemon_ExitsWhenTheRepositoryIsRemoved(t *testing.T) {
	root := cacheProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	socket := daemonSocketForTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	daemon, err := StartDaemon(ctx, &ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: t.TempDir()},
		DaemonOptions{SocketPath: socket, IdleTimeout: 200 * time.Millisecond})
	require.NoError(t, err)
	t.Cleanup(daemon.Close)

	require.NoError(t, os.RemoveAll(root))

	select {
	case <-daemon.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the daemon kept running after its repository was removed")
	}
}

func TestPinWorkDir_MakesPathsAbsolute(t *testing.T) {
	st := &ServeSetup{CacheDir: "cache", UsageLog: "usage.jsonl"}

	st.pinWorkDir()

	cwd, err := os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, cwd, st.WorkDir)
	assert.Equal(t, filepath.Join(cwd, "cache"), st.CacheDir)
	assert.Equal(t, filepath.Join(cwd, "usage.jsonl"), st.UsageLog)
	assert.Empty(t, st.ServeCacheDir)
}

// The daemon leaves the repository directory as its working directory before it
// builds anything, so a worktree can be removed while it runs (#481).
func TestRunDaemon_DoesNotKeepTheRepositoryAsItsWorkingDirectory(t *testing.T) {
	root := cacheProject(t)
	socket := daemonSocketForTest(t)
	t.Chdir(root)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- RunDaemon(ctx, &ServeSetup{NoWatch: true, ServeCacheDir: t.TempDir()}, DaemonOptions{SocketPath: socket, IdleTimeout: time.Minute, Chdir: os.Chdir})
	}()

	require.Eventually(t, func() bool {
		conn, err := dialDaemon(socket)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 30*time.Second, 50*time.Millisecond)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	resolvedDir, err := filepath.EvalSymlinks(filepath.Dir(socket))
	require.NoError(t, err)
	assert.Equal(t, resolvedDir, cwd)
	cancel()
	require.NoError(t, <-done)
}
