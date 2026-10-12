//go:build !windows

package mcp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// daemonSocketForTest is a socket path in a private temporary directory, so the
// tests never touch the real user cache. It avoids t.TempDir, whose path is long
// enough to exceed the unix socket path limit.
func daemonSocketForTest(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ard-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, daemonSocketName)
}

// proxySkills runs one stdio proxy against socket and returns the served skill
// names, as a client that only speaks tools sees them.
func proxySkills(t *testing.T, socket, root string) []string {
	t.Helper()
	return proxySkillsWith(t, root, ProxyOptions{SocketPath: socket})
}

// proxySkillsWith is proxySkills with the proxy options spelled out, so a test
// can exercise the daemon-starting path.
func proxySkillsWith(t *testing.T, root string, opts ProxyOptions) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	// proxyIn is what the client writes, proxyOut what the proxy writes back.
	proxyIn, clientOut := io.Pipe()
	clientIn, proxyOut := io.Pipe()
	setup := &ServeSetup{WorkDir: root, NoWatch: true}
	go func() {
		_, _ = ServeViaDaemon(ctx, setup, proxyIn, proxyOut, opts)
	}()
	session, err := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "c", Version: "1"}, nil).
		Connect(ctx, &sdkmcp.IOTransport{Reader: clientIn, Writer: clientOut}, nil)
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "search_skills", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.False(t, res.IsError)
	out, ok := res.StructuredContent.(map[string]any)
	require.True(t, ok)
	names := make([]string, 0, len(out["results"].([]any)))
	for _, r := range out["results"].([]any) {
		names = append(names, r.(map[string]any)["name"].(string))
	}
	sort.Strings(names)
	return names
}

// Two stdio proxies that connect to the same socket are served by one daemon:
// the daemon renders the catalog once and each proxy opens its own session.
func TestDaemon_TwoProxiesShareOneDaemon(t *testing.T) {
	root := cacheProject(t)
	socket := daemonSocketForTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	daemon, err := StartDaemon(ctx, &ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: t.TempDir()},
		DaemonOptions{SocketPath: socket, IdleTimeout: time.Minute})
	require.NoError(t, err)
	t.Cleanup(daemon.Close)

	assert.Equal(t, []string{"a", "b"}, proxySkills(t, socket, root))
	assert.Equal(t, []string{"a", "b"}, proxySkills(t, socket, root))
	assert.Equal(t, int64(2), daemon.Sessions(), "both proxies were served by the one daemon")
}

// A content change is picked up by the daemon's live reload, so a session that
// connects after the edit sees the new catalog without a restart.
func TestDaemon_ContentChangeIsPickedUp(t *testing.T) {
	root := cacheProject(t)
	socket := daemonSocketForTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	daemon, err := StartDaemon(ctx, &ServeSetup{WorkDir: root, ServeCacheDir: t.TempDir(), PollInterval: 20 * time.Millisecond},
		DaemonOptions{SocketPath: socket, IdleTimeout: time.Minute})
	require.NoError(t, err)
	t.Cleanup(daemon.Close)
	require.Equal(t, []string{"a", "b"}, proxySkills(t, socket, root))

	writeFile(t, root, ".ai-rulez/skills/c/SKILL.md", skillFile("c", "Skill c", ""))

	require.Eventually(t, func() bool {
		return len(proxySkills(t, socket, root)) == 3
	}, 15*time.Second, 50*time.Millisecond)
	assert.Equal(t, []string{"a", "b", "c"}, proxySkills(t, socket, root))
}

// With no client, the daemon exits after the idle timeout and removes its socket.
func TestDaemon_IdleTimeoutExits(t *testing.T) {
	root := cacheProject(t)
	socket := daemonSocketForTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	daemon, err := StartDaemon(ctx, &ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: t.TempDir()},
		DaemonOptions{SocketPath: socket, IdleTimeout: 100 * time.Millisecond})
	require.NoError(t, err)
	stopped := make(chan struct{})
	go func() {
		daemon.Wait()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("the daemon did not exit after the idle timeout")
	}
	_, statErr := os.Lstat(socket)
	assert.True(t, os.IsNotExist(statErr), "the socket is removed on exit")
}

// The first proxy starts the daemon through the spawner hook and then connects
// to it: nothing listens on the socket before the session begins.
func TestDaemon_ProxyStartsTheDaemonOnFirstUse(t *testing.T) {
	root := cacheProject(t)
	socket := daemonSocketForTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	started := make(chan *Daemon, 1)
	start := func(path string) error {
		daemon, err := StartDaemon(ctx, &ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: t.TempDir()},
			DaemonOptions{SocketPath: path, IdleTimeout: time.Minute})
		if err != nil {
			return err
		}
		started <- daemon
		return nil
	}
	assert.Equal(t, []string{"a", "b"}, proxySkillsWith(t, root, ProxyOptions{SocketPath: socket, StartDaemon: start}))

	select {
	case daemon := <-started:
		t.Cleanup(daemon.Close)
		assert.Equal(t, int64(1), daemon.Sessions())
	case <-time.After(time.Second):
		t.Fatal("the proxy did not start a daemon")
	}
}

// The socket is 0600 inside a 0700 directory, so only its owner can reach it.
func TestDaemon_SocketAndDirectoryPermissions(t *testing.T) {
	root := cacheProject(t)
	socket := daemonSocketForTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	daemon, err := StartDaemon(ctx, &ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: t.TempDir()},
		DaemonOptions{SocketPath: socket, IdleTimeout: time.Minute})
	require.NoError(t, err)
	t.Cleanup(daemon.Close)

	socketInfo, err := os.Lstat(socket)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), socketInfo.Mode().Perm(), "the socket is private")
	dirInfo, err := os.Stat(filepath.Dir(socket))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm(), "the socket directory is private")
}

// A daemon of another version refuses the session, which falls back in-process.
func TestDaemon_VersionMismatchFallsBack(t *testing.T) {
	root := cacheProject(t)
	socket := daemonSocketForTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	daemon, err := StartDaemon(ctx, &ServeSetup{WorkDir: root, Version: "1.0.0", NoWatch: true, ServeCacheDir: t.TempDir()},
		DaemonOptions{SocketPath: socket, IdleTimeout: time.Minute})
	require.NoError(t, err)
	defer daemon.Close()

	handled, err := ServeViaDaemon(ctx, &ServeSetup{WorkDir: root, Version: "2.0.0"},
		bytes.NewReader(nil), io.Discard, ProxyOptions{SocketPath: socket})
	require.NoError(t, err)
	assert.False(t, handled, "a version mismatch must fall back in-process")
}

// A daemon serving another view refuses the session, which falls back in-process
// rather than being served the wrong skills.
func TestDaemon_ViewMismatchFallsBack(t *testing.T) {
	root := cacheProject(t)
	socket := daemonSocketForTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	daemon, err := StartDaemon(ctx, &ServeSetup{WorkDir: root, NoWatch: true, IncludeStatic: true, ServeCacheDir: t.TempDir()},
		DaemonOptions{SocketPath: socket, IdleTimeout: time.Minute})
	require.NoError(t, err)
	defer daemon.Close()

	handled, err := ServeViaDaemon(ctx, &ServeSetup{WorkDir: root}, // the default view, not the daemon's "static" view
		bytes.NewReader(nil), io.Discard, ProxyOptions{SocketPath: socket})
	require.NoError(t, err)
	assert.False(t, handled, "a view mismatch must fall back in-process")
}

// A daemon that loses the start race to another one does not take over the
// socket.
func TestDaemon_SecondStartLosesTheRace(t *testing.T) {
	root := cacheProject(t)
	socket := daemonSocketForTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	first, err := StartDaemon(ctx, &ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: t.TempDir()},
		DaemonOptions{SocketPath: socket, IdleTimeout: time.Minute})
	require.NoError(t, err)
	defer first.Close()

	_, err = StartDaemon(ctx, &ServeSetup{WorkDir: root, NoWatch: true, ServeCacheDir: t.TempDir()},
		DaemonOptions{SocketPath: socket})
	assert.ErrorIs(t, err, errDaemonAlreadyRunning)
}

// A missing or broken daemon never fails a session: ServeViaDaemon reports
// handled false so the caller serves in-process.
func TestDaemon_MissingOrBrokenSocketFallsBackInProcess(t *testing.T) {
	root := cacheProject(t)
	regularFile := func(t *testing.T) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "file.sock")
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o600))
		return p
	}
	tests := []struct {
		name   string
		socket func(t *testing.T) string
		start  func(string) error
	}{
		{
			name:   "no daemon and no spawner",
			socket: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.sock") },
		},
		{
			name:   "the spawner fails",
			socket: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.sock") },
			start:  func(string) error { return errors.New("cannot start the daemon") },
		},
		{
			name:   "the socket path is a regular file",
			socket: regularFile,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handled, err := ServeViaDaemon(context.Background(), &ServeSetup{WorkDir: root},
				bytes.NewReader(nil), io.Discard,
				ProxyOptions{SocketPath: tt.socket(t), StartDaemon: tt.start, StartWait: 200 * time.Millisecond})
			require.NoError(t, err)
			assert.False(t, handled, "the caller must serve in-process")
		})
	}
}

// A socket that another user could reach is refused and never replaced by a
// daemon of ours.
func TestDaemon_UnsafeSocketIsRefused(t *testing.T) {
	root := cacheProject(t)
	socket := daemonSocketForTest(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(socket), 0o700))
	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	require.NoError(t, os.Chmod(socket, 0o666))

	handled, err := ServeViaDaemon(context.Background(), &ServeSetup{WorkDir: root},
		bytes.NewReader(nil), io.Discard,
		ProxyOptions{SocketPath: socket, StartDaemon: func(string) error {
			t.Fatal("a daemon must not be started over an unsafe socket")
			return nil
		}})
	require.NoError(t, err)
	assert.False(t, handled)
	_, statErr := os.Lstat(socket)
	assert.NoError(t, statErr, "the unsafe socket is left alone")
}

// The socket path is keyed by the repository toplevel, the version and the
// setup's identity, so a version or an option change never shares a daemon and
// two subdirectories of one checkout do.
func TestDaemonSocketPath_KeyedByRepoVersionAndIdentity(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "nested", "deeper"), 0o755))

	base := &ServeSetup{Version: "5.0.3", WorkDir: root, DaemonIdentity: "one"}
	basePath, err := base.daemonSocketPath()
	require.NoError(t, err)

	nested, err := (&ServeSetup{Version: "5.0.3", WorkDir: filepath.Join(root, "nested", "deeper"), DaemonIdentity: "one"}).daemonSocketPath()
	require.NoError(t, err)
	assert.Equal(t, basePath, nested, "a subdirectory shares the repository's daemon")

	otherIdentity, err := (&ServeSetup{Version: "5.0.3", WorkDir: root, DaemonIdentity: "two"}).daemonSocketPath()
	require.NoError(t, err)
	assert.NotEqual(t, basePath, otherIdentity, "a different setup identity is a different daemon")

	otherVersion, err := (&ServeSetup{Version: "5.0.4", WorkDir: root, DaemonIdentity: "one"}).daemonSocketPath()
	require.NoError(t, err)
	assert.NotEqual(t, basePath, otherVersion, "a version change is a different daemon")
}

// The derived identity changes with the setup, so a library caller that names no
// identity still never shares a daemon with a different view.
func TestDerivedDaemonIdentity_ChangesWithTheSetup(t *testing.T) {
	base := (&ServeSetup{Profile: "backend"}).derivedDaemonIdentity()
	assert.Equal(t, base, (&ServeSetup{Profile: "backend"}).derivedDaemonIdentity())
	assert.NotEqual(t, base, (&ServeSetup{Profile: "frontend"}).derivedDaemonIdentity())
	assert.NotEqual(t, base, (&ServeSetup{Profile: "backend", Filter: SkillFilter{Deny: []string{"x"}}}).derivedDaemonIdentity())
}
