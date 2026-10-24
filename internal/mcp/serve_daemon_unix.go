//go:build !windows

package mcp

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/samber/oops"
)

// daemonSupported reports whether the shared daemon is available here. Unix
// domain sockets carry the file permissions the daemon's security model needs
// (owner, 0600 in a 0700 directory); the Windows path serves in-process.
func daemonSupported() bool { return true }

// listenDaemon binds the daemon socket, refusing a socket that is not ours and
// replacing a stale one. It returns errDaemonAlreadyRunning when a live daemon
// owns the socket, so the loser of a start race exits quietly.
func listenDaemon(path string) (net.Listener, error) {
	if err := ensureDaemonDir(path); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, oops.Errorf("the shared daemon socket path %s exists and is not a socket", path)
		}
		live, _ := socketLive(path) //nolint:errcheck // a probe that fails means the socket is not live
		if live {
			return nil, errDaemonAlreadyRunning
		}
		if err := socketOwnerOK(path); err != nil {
			return nil, err
		}
		if err := os.Remove(path); err != nil {
			return nil, oops.Wrapf(err, "remove the stale shared daemon socket")
		}
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, oops.Wrapf(err, "listen on the shared daemon socket")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close() //nolint:errcheck // the chmod error is what matters
		return nil, oops.Wrapf(err, "restrict the shared daemon socket")
	}
	return listener, nil
}

// dialDaemon connects to the daemon socket. It refuses a socket that is not ours
// (errDaemonUnsafe) and reports a missing or stale one as errDaemonMissing, which
// the caller may answer by starting a daemon.
func dialDaemon(path string) (net.Conn, error) {
	if _, err := os.Lstat(path); err != nil {
		return nil, errDaemonMissing
	}
	if err := socketOwnerOK(path); err != nil {
		return nil, errors.Join(errDaemonUnsafe, err)
	}
	conn, err := net.DialTimeout("unix", path, daemonConnectTimeout)
	if err != nil {
		return nil, errDaemonMissing
	}
	return conn, nil
}

// ensureDaemonDir prepares the private directory that holds socket. A socket
// placed under the shared runtime or temp directory (shortSocketName) also needs
// its per-user parent private: another user who created that parent first could
// otherwise replace the directory or plant links in it.
func ensureDaemonDir(socket string) error {
	dir := filepath.Dir(socket)
	if filepath.Base(socket) == shortSocketName {
		if err := ensureSocketDir(filepath.Dir(dir)); err != nil {
			return err
		}
	}
	return ensureSocketDir(dir)
}

// maxDaemonLogBytes is the size past which a new daemon starts the log afresh.
const maxDaemonLogBytes = 1 << 20

// OpenDaemonLog opens daemon.log in the socket's private directory, for the
// daemon's stderr. It returns nil (stderr stays on the null device) when the
// directory is not private to this user or the log cannot be opened, and never
// follows a link at the log's path.
func OpenDaemonLog(socket string) *os.File {
	if ensureDaemonDir(socket) != nil {
		return nil
	}
	path := filepath.Join(filepath.Dir(socket), "daemon.log")
	flags := os.O_WRONLY | os.O_CREATE | os.O_APPEND | syscall.O_NOFOLLOW
	if info, err := os.Lstat(path); err == nil && info.Size() > maxDaemonLogBytes {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o600) //nolint:gosec // a fixed name in the daemon's private directory
	if err != nil {
		return nil
	}
	return f
}

// ensureSocketDir creates the socket's directory 0700 and refuses one group or
// others can reach, so no other user can see or connect to the socket.
func ensureSocketDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return oops.Wrapf(err, "create the shared daemon directory")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return oops.Wrapf(err, "inspect the shared daemon directory")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return oops.Errorf("the shared daemon directory %s is accessible by group or others", dir)
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && int(stat.Uid) != os.Getuid() {
		return oops.Errorf("the shared daemon directory %s is owned by uid %d, not %d", dir, stat.Uid, os.Getuid())
	}
	return nil
}

// socketLive reports whether something is accepting connections on path.
func socketLive(path string) (bool, error) {
	conn, err := net.DialTimeout("unix", path, 250*time.Millisecond)
	if err != nil {
		return false, err //nolint:wrapcheck // a refused or missing socket is the answer
	}
	_ = conn.Close() //nolint:errcheck // the probe is done
	return true, nil
}

// socketOwnerOK reports whether path is a socket owned by this user and not
// reachable by group or others.
func socketOwnerOK(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err //nolint:wrapcheck // the caller reports the path
	}
	if info.Mode()&os.ModeSocket == 0 {
		return oops.Errorf("the shared daemon socket %s is not a socket", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return oops.Errorf("the shared daemon socket %s is accessible by group or others", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if uid := os.Getuid(); int(stat.Uid) != uid {
		return oops.Errorf("the shared daemon socket %s is owned by uid %d, not %d", path, stat.Uid, uid)
	}
	return nil
}
