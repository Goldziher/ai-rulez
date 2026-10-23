//go:build !windows

package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/samber/oops"
)

// startDaemonProcess spawns the shared daemon: the same command line in a new
// session, with stdin and stdout on the null device so it never touches the MCP
// pipe and stderr in daemon.log beside the socket, where its warnings (a watch
// budget reached, an exit and why) can be read. The child inherits the working directory and derives the same
// socket as its proxy. It is detached from the proxy's session, so it outlives
// the session that started it and serves later ones.
func startDaemonProcess(socket string) error {
	exe, err := os.Executable()
	if err != nil {
		return oops.Wrapf(err, "locate the ai-rulez binary to start the shared daemon")
	}
	args := append(append([]string(nil), os.Args[1:]...), "--"+flagServeDaemonServer)
	cmd := exec.Command(exe, args...) //nolint:gosec // the binary's own command line
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if logFile := openDaemonLog(socket); logFile != nil {
		defer logFile.Close() //nolint:errcheck // the child holds its own descriptor
		cmd.Stderr = logFile
	}
	if err := cmd.Start(); err != nil {
		return oops.Wrapf(err, "start the shared daemon")
	}
	// Reap the child when it exits, so it does not become a zombie while the
	// proxy is still alive; if the proxy exits first, init adopts it.
	go func() { _ = cmd.Wait() }() //nolint:errcheck // the exit status is not the proxy's concern
	return nil
}

// maxDaemonLogBytes is the size past which a new daemon starts the log afresh.
const maxDaemonLogBytes = 1 << 20

// openDaemonLog opens daemon.log in the socket's private directory, or returns
// nil (stderr stays on the null device) when it cannot.
func openDaemonLog(socket string) *os.File {
	dir := filepath.Dir(socket)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_APPEND
	if info, err := os.Stat(filepath.Join(dir, "daemon.log")); err == nil && info.Size() > maxDaemonLogBytes {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(filepath.Join(dir, "daemon.log"), flags, 0o600)
	if err != nil {
		return nil
	}
	return f
}
