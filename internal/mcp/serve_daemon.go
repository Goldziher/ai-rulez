package mcp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/samber/oops"
)

// The shared per-repo daemon. Building the served catalog renders every skill
// and scans it, which on a large monorepo takes seconds; the on-disk catalog
// cache (serve_cache.go) removes the cost of every restart after the first, but
// each session still pays its own config load and holds its own copy of the
// catalog and the search index in memory. With --daemon one long-lived process
// per repository renders the catalog once and serves it to every stdio proxy
// over a private unix socket: the first session pays the build, every later
// session connects in milliseconds.
//
// The daemon is opt-in and additive. A stdio proxy connects to the socket, and
// on ANY failure (no daemon, an unsafe or broken socket, a version or view
// mismatch) it serves the session in-process exactly as without --daemon, so a
// broken daemon never fails a session. The socket lives in a 0700 directory
// under the user cache and is chmod 0600; a proxy refuses a socket owned by
// another user or accessible by group or others. The daemon exits after
// DefaultDaemonIdleTimeout with no connected client and removes its socket. On
// Windows the daemon is not offered and the server serves in-process.
const (
	// DefaultDaemonIdleTimeout is how long the shared daemon stays alive after
	// its last client disconnected.
	DefaultDaemonIdleTimeout = 15 * time.Minute
	// daemonConnectTimeout bounds one connection attempt to a daemon.
	daemonConnectTimeout = 2 * time.Second
	// daemonStartWait bounds how long a proxy waits for a spawned daemon to bind
	// its socket (the bind happens before the catalog is built, so this covers
	// process start only).
	daemonStartWait = 10 * time.Second
	// daemonHandshakeTimeout bounds the hello exchange. The daemon binds its
	// socket before it builds the catalog and answers the hello only once the
	// catalog is ready, so this must cover a cold build — the same work the
	// session would otherwise do in-process. It matches the MCP_TIMEOUT the docs
	// recommend for a cold start, and a build that takes longer than this would
	// have blown the client's own timeout anyway.
	daemonHandshakeTimeout = 60 * time.Second
	// daemonProtocol is the version of the hello the two ends exchange. It is
	// bumped when the wire shape changes.
	daemonProtocol = 1
	// daemonSocketName is the socket file inside the daemon's private directory.
	daemonSocketName = "daemon.sock"
	// shortSocketName is the socket file when the path has to be kept short.
	shortSocketName = "d.sock"
	// maxDaemonSocketPath bounds the socket path on macOS and the BSDs, whose
	// unix socket paths are limited to 104 bytes; Linux allows 108 (see
	// socketPathLimit). A longer path is moved to a short runtime directory, and
	// refused when that is too long too, instead of failing with a raw bind error.
	maxDaemonSocketPath = 100
	// maxDaemonSocketPathLinux is the same bound where the limit is 108 bytes.
	maxDaemonSocketPathLinux = 104
)

// socketPathLimit is the longest socket path the platform binds, with a margin.
func socketPathLimit() int {
	if runtime.GOOS == "linux" {
		return maxDaemonSocketPathLinux
	}
	return maxDaemonSocketPath
}

// Errors of the daemon transport. A caller treats all of them as "serve
// in-process"; errDaemonUnsafe is the one a proxy must not answer by starting a
// daemon over the socket.
var (
	errDaemonUnsupported    = errors.New("the shared MCP daemon is not supported on this platform")
	errDaemonMissing        = errors.New("no shared MCP daemon is listening")
	errDaemonUnsafe         = errors.New("the shared MCP daemon socket is not safe to use")
	errDaemonAlreadyRunning = errors.New("a shared MCP daemon is already listening")
)

// DaemonOptions configures StartDaemon and RunDaemon.
type DaemonOptions struct {
	// IdleTimeout is how long the daemon stays alive after its last client
	// disconnected; 0 selects DefaultDaemonIdleTimeout.
	IdleTimeout time.Duration
	// SocketPath overrides the socket derived from the setup (tests).
	SocketPath string
	// Chdir moves the process into the socket's private directory before the
	// daemon builds, so a daemon that outlives its client does not keep the
	// checkout as its working directory. nil stays put; the CLI passes os.Chdir.
	Chdir func(dir string) error
}

// ProxyOptions configures ServeViaDaemon.
type ProxyOptions struct {
	// SocketPath overrides the socket derived from the setup (tests).
	SocketPath string
	// StartDaemon starts a daemon listening on the socket. nil never starts one,
	// so ServeViaDaemon only connects to a daemon that is already running and
	// falls back when none is. The CLI supplies a spawner; tests supply a fake
	// or nil.
	StartDaemon func(socket string) error
	// StartWait bounds how long to wait for a spawned daemon to listen; 0
	// selects daemonStartWait.
	StartWait time.Duration
}

// Daemon is a running shared daemon: one rendered catalog served to many stdio
// proxies over a private unix socket.
type Daemon struct {
	socketPath string
	// root is the repository the daemon serves; the daemon exits when it is gone.
	root     string
	listener net.Listener
	server   *Server
	// view is the serve view the daemon serves (ServeSetup.ViewKey); a client
	// whose view differs is refused rather than served another view's skills.
	view string

	sessions atomic.Int64
	active   atomic.Int64
	lastSeen atomic.Int64 // unix nanoseconds of the last connect or disconnect

	done      chan struct{}
	closeOnce sync.Once
	serverOne sync.Once
}

// SocketPath is where the daemon listens.
func (d *Daemon) SocketPath() string { return d.socketPath }

// Sessions is how many MCP sessions the daemon has served since it started.
func (d *Daemon) Sessions() int64 { return d.sessions.Load() }

// Active is how many clients are connected right now.
func (d *Daemon) Active() int64 { return d.active.Load() }

// StartDaemon builds the setup's server and serves it on the shared daemon
// socket until ctx ends or the idle timeout elapses with no client connected. It
// returns once the daemon is listening; call Wait for it to stop, or Close to
// stop it. errDaemonAlreadyRunning means another daemon owns the socket.
func StartDaemon(ctx context.Context, st *ServeSetup, opts DaemonOptions) (*Daemon, error) {
	if !daemonSupported() {
		return nil, errDaemonUnsupported
	}
	path := opts.SocketPath
	if path == "" {
		derived, err := st.daemonSocketPath()
		if err != nil {
			return nil, err
		}
		path = derived
	}
	listener, err := listenDaemon(path)
	if err != nil {
		return nil, err //nolint:wrapcheck // already names the socket
	}
	server, err := st.NewServer(ctx)
	if err != nil {
		_ = listener.Close() //nolint:errcheck // best-effort cleanup of a daemon that never started
		_ = os.Remove(path)  //nolint:errcheck // best-effort cleanup of a daemon that never started
		return nil, oops.Wrapf(err, "build the shared daemon's skills server")
	}
	idle := opts.IdleTimeout
	if idle <= 0 {
		idle = st.DaemonIdleTimeout
	}
	if idle <= 0 {
		idle = DefaultDaemonIdleTimeout
	}
	d := &Daemon{
		socketPath: path, listener: listener, server: server, view: st.ViewKey(), done: make(chan struct{}),
		root: repoTopLevel(st.workDir()),
	}
	d.touch()
	go server.Watch(ctx)
	go d.serve(ctx, idle)
	return d, nil
}

// RunDaemon starts the shared daemon and blocks until it stops: the idle timeout
// elapsed with no client, or ctx ended. A start that lost the race to another
// daemon returns at once, so the spawned child exits cleanly.
func RunDaemon(ctx context.Context, st *ServeSetup, opts DaemonOptions) error {
	// The socket is derived before the paths are pinned: the derived identity
	// hashes them as the proxy saw them.
	if opts.SocketPath == "" {
		path, err := st.daemonSocketPath()
		if err != nil {
			return err
		}
		opts.SocketPath = path
	}
	st.pinWorkDir()
	// The daemon outlives its client, so it must not keep the checkout (often a
	// throwaway worktree) as its working directory, not even while it builds.
	if opts.Chdir != nil && ensureDaemonDir(opts.SocketPath) == nil {
		if err := opts.Chdir(filepath.Dir(opts.SocketPath)); err != nil {
			logger.Or(nil).Warn("MCP daemon: could not leave the repository directory", "error", err.Error())
		}
	}
	pruneStaleDaemonDirsBeside(opts.SocketPath, st.Clock.Now())
	d, err := StartDaemon(ctx, st, opts)
	if err != nil {
		if errors.Is(err, errDaemonAlreadyRunning) {
			return nil
		}
		return err //nolint:wrapcheck // already contextual
	}
	d.Wait()
	return nil
}

// pinWorkDir makes the paths the daemon reads later (live reload, the watcher)
// absolute, so they survive a change of the process's working directory.
func (st *ServeSetup) pinWorkDir() {
	abs := func(p string) string {
		if p == "" {
			return p
		}
		if a, err := filepath.Abs(p); err == nil {
			return a
		}
		return p
	}
	st.WorkDir = abs(st.workDir())
	st.CacheDir = abs(st.CacheDir)
	st.ServeCacheDir = abs(st.ServeCacheDir)
	if st.UsageLog != "-" {
		st.UsageLog = abs(st.UsageLog)
	}
}

// Wait blocks until the daemon stops and then releases the server's background
// work (the usage sink, the search runtime).
func (d *Daemon) Wait() {
	<-d.done
	d.serverOne.Do(func() {
		if d.server != nil {
			d.server.Close()
		}
	})
}

// Close stops the daemon: it closes the listener and removes the socket. A
// client connecting after Close is refused and falls back in-process.
func (d *Daemon) Close() {
	d.closeOnce.Do(func() {
		_ = d.listener.Close()      //nolint:errcheck // closing the listener is the point
		_ = os.Remove(d.socketPath) //nolint:errcheck // best-effort removal of the socket
	})
}

// serve accepts clients until the listener is closed. Each client is served on
// its own goroutine, so many sessions share the one catalog.
func (d *Daemon) serve(ctx context.Context, idle time.Duration) {
	defer close(d.done)
	go d.watchIdle(ctx, idle)
	for {
		conn, err := d.listener.Accept()
		if err != nil {
			return
		}
		d.active.Add(1)
		d.touch()
		go d.handle(ctx, conn)
	}
}

// handle serves one client: the hello, then the MCP session over the same
// guarded transport the in-process server uses.
func (d *Daemon) handle(ctx context.Context, conn net.Conn) {
	defer func() {
		d.active.Add(-1)
		d.touch()
	}()
	defer conn.Close() //nolint:errcheck // the session owns the connection
	log := logger.Or(nil)
	reader := bufio.NewReader(conn)
	if err := acceptDaemonClient(reader, conn, d.server, d.view); err != nil {
		log.Debug("MCP daemon: refused a client", "error", err.Error())
		return
	}
	d.sessions.Add(1)
	// End the session when the daemon's context ends, so a signal does not leave
	// a connection hanging.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close() //nolint:errcheck // shutting the session down is the point
		case <-stop:
		}
	}()
	session, err := d.server.GetMCPServer().Connect(ctx, d.server.WrapTransport(newGuardedTransport(reader, conn, MaxFrameBytes, log)), nil)
	if err != nil {
		log.Debug("MCP daemon: a session did not start", "error", err.Error())
		return
	}
	_ = session.Wait() //nolint:errcheck // a session ending is not the daemon's error
}

// watchIdle stops the daemon once it has had no client for idle. It closes the
// listener first, so a client connecting at that moment is refused and falls
// back in-process rather than reaching a daemon that is leaving.
func (d *Daemon) watchIdle(ctx context.Context, idle time.Duration) {
	interval := idle / 4
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	if interval > time.Minute {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			d.Close()
			return
		case <-ticker.C:
			if _, err := os.Stat(d.root); errors.Is(err, fs.ErrNotExist) {
				logger.Or(nil).Info("MCP daemon: exiting because the repository is gone", "root", d.root)
				d.Close()
				return
			}
			if d.active.Load() != 0 {
				continue
			}
			if ambient.Clock(nil).Now().Sub(d.lastActiveTime()) < idle {
				continue
			}
			logger.Or(nil).Info("MCP daemon: exiting after the idle timeout", "idle", idle.String())
			d.Close()
			return
		}
	}
}

// touch records the current time as the last client activity.
func (d *Daemon) touch() { d.lastSeen.Store(ambient.Clock(nil).Now().UnixNano()) }

// lastActiveTime is when a client last connected or disconnected.
func (d *Daemon) lastActiveTime() time.Time { return time.Unix(0, d.lastSeen.Load()) }

// daemonSocketPath is where the setup's daemon listens: a private directory
// under the user cache, keyed by the repository toplevel, the ai-rulez version
// and the setup's identity, so a version change, a different view or a different
// authoring root never shares a socket. A content change does not start a new
// daemon: the running one reloads it (serve_live.go), which is what keeps one
// daemon per repository rather than one per content state.
func (st *ServeSetup) daemonSocketPath() (string, error) {
	root := repoTopLevel(st.workDir())
	identity := st.DaemonIdentity
	if identity == "" {
		identity = st.derivedDaemonIdentity()
	}
	sum := sha256.Sum256([]byte(root + "\x00" + st.Version + "\x00" + identity))
	dir, err := config.CacheDir("daemons", hex.EncodeToString(sum[:8]))
	if err != nil {
		return "", oops.Wrapf(err, "place the shared daemon socket")
	}
	path := filepath.Join(dir, daemonSocketName)
	limit := socketPathLimit()
	if len(path) <= limit {
		return path, nil
	}
	// A long home (a per-session sandbox, a CI workspace) cannot hold the socket;
	// a per-user directory under the runtime or temp directory, keyed by the same
	// hash, can. ensureSocketDir refuses it unless this user owns it with mode 0700.
	short := filepath.Join(shortSocketBase(), fmt.Sprintf("ai-rulez-%d", os.Getuid()), hex.EncodeToString(sum[:6]), shortSocketName)
	if len(short) <= limit {
		return short, nil
	}
	return "", oops.Hint("Set a shorter home directory or TMPDIR, or run without --daemon").
		Errorf("the shared daemon socket path is %d bytes (%d under the temp directory), over the %d-byte limit", len(path), len(short), limit)
}

// shortSocketBase is where a socket goes when the cache path is too long:
// XDG_RUNTIME_DIR when set (a per-user 0700 directory), else the temp directory.
func shortSocketBase() string {
	if dir := ambient.Getenv(nil, "XDG_RUNTIME_DIR"); dir != "" {
		return dir
	}
	return os.TempDir()
}

// derivedDaemonIdentity hashes the setup fields that change what the daemon
// serves, for a caller that named no identity (library users and tests). The
// authoring options are not visible here, so the CLI always names one.
func (st *ServeSetup) derivedDaemonIdentity() string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%t\x00%t\x00%t\x00%t\x00%d\x00%d\x00%q\x00%q\x00", //nolint:errcheck,gosec // hash writes never fail
		st.Profile, st.Preset, st.Role, st.IncludeStatic, st.Frozen, st.Offline, st.ServeUserInvoked,
		st.MaxCloneBytes, st.BudgetBytes, st.UsageLog, st.UsageSink)
	writeKeyStrings(h, st.Filter.Domains)
	writeKeyStrings(h, st.Filter.Allow)
	writeKeyStrings(h, st.Filter.Deny)
	writeKeyStrings(h, st.Sources)
	return hex.EncodeToString(h.Sum(nil))
}

// repoTopLevel is the repository root above dir: the nearest ancestor holding a
// .git entry (a directory, or a file in a linked worktree), or dir itself. It is
// what makes two sessions in different subdirectories of one checkout share a
// daemon, while a subagent worktree gets its own.
func repoTopLevel(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = filepath.Clean(dir)
	}
	if resolved, rerr := filepath.EvalSymlinks(abs); rerr == nil {
		abs = resolved // /var and /private/var name one project
	}
	for p := abs; ; {
		if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			return abs
		}
		p = parent
	}
}

// daemonHello is the proxy's greeting: the protocol and version it speaks and
// the view it wants.
type daemonHello struct {
	Protocol int    `json:"protocol"`
	Version  string `json:"version"`
	View     string `json:"view,omitempty"`
}

// daemonHelloResp is the daemon's answer. A refusal carries ok false and the
// reason; the proxy then serves in-process.
type daemonHelloResp struct {
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
	View    string `json:"view,omitempty"`
	Skills  int    `json:"skills,omitempty"`
	Error   string `json:"error,omitempty"`
}

// acceptDaemonClient reads one hello and answers it. The reader is the one the
// caller hands to the session transport, so a frame the client sent right after
// its hello is not lost. A hello whose protocol, version or view is not this
// daemon's is refused, and the client falls back in-process rather than being
// served another view's skills.
func acceptDaemonClient(reader *bufio.Reader, conn net.Conn, server *Server, view string) error {
	if err := conn.SetDeadline(ambient.Clock(nil).Now().Add(daemonHandshakeTimeout)); err != nil {
		return oops.Wrapf(err, "bound the daemon handshake")
	}
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return oops.Wrapf(err, "read the daemon hello")
	}
	var hello daemonHello
	if err := json.Unmarshal(bytes.TrimSpace(line), &hello); err != nil {
		_ = writeDaemonLine(conn, daemonHelloResp{Error: "malformed hello"}) //nolint:errcheck // the parse error is what matters
		return oops.Wrapf(err, "parse the daemon hello")
	}
	resp := daemonHelloResp{OK: true, Version: server.version, View: view}
	switch {
	case hello.Protocol != daemonProtocol:
		resp = daemonHelloResp{Error: fmt.Sprintf("unknown daemon protocol %d", hello.Protocol)}
	case hello.Version != server.version:
		resp = daemonHelloResp{Error: fmt.Sprintf("the daemon serves version %s, not %s", server.version, hello.Version)}
	case hello.View != view:
		resp = daemonHelloResp{Error: fmt.Sprintf("the daemon serves the view %q, not %q", view, hello.View)}
	default:
		resp.Skills = len(server.Catalog().Skills())
	}
	if err := writeDaemonLine(conn, resp); err != nil {
		return err
	}
	if !resp.OK {
		return oops.Errorf("the daemon refused the client: %s", resp.Error)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return oops.Wrapf(err, "clear the daemon handshake deadline")
	}
	return nil
}

// writeDaemonLine writes one newline-terminated JSON object.
func writeDaemonLine(conn net.Conn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return oops.Wrapf(err, "encode the daemon reply")
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return oops.Wrapf(err, "write the daemon reply")
	}
	return nil
}

// pumpDaemon copies the client's bytes to the daemon and the daemon's bytes back
// until either side closes. It frames nothing: both ends speak
// newline-delimited JSON-RPC, and the daemon runs the same guarded transport the
// in-process server does.
func pumpDaemon(ctx context.Context, conn net.Conn, fromDaemon io.Reader, in io.Reader, out io.Writer) {
	toClient := make(chan struct{})
	go func() {
		defer close(toClient)
		_, _ = io.Copy(out, fromDaemon) //nolint:errcheck // a copy error ends the proxy, which is the point
	}()
	go func() {
		_, _ = io.Copy(conn, in) //nolint:errcheck // a copy error ends the proxy, which is the point
		halfCloseWrite(conn)
	}()
	select {
	case <-ctx.Done():
	case <-toClient:
	}
	_ = conn.Close() //nolint:errcheck // ending the proxy is the point
}

// halfCloseWrite ends the write half of conn so the daemon sees EOF, keeping the
// read half open until the daemon's reply is drained.
func halfCloseWrite(conn net.Conn) {
	if closer, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = closer.CloseWrite() //nolint:errcheck // best-effort half close
		return
	}
	_ = conn.Close() //nolint:errcheck // best-effort close
}
