//go:build windows

package mcp

import "net"

// daemonSupported is false on Windows: the shared daemon is built on unix domain
// socket permissions (owner, 0600 in a 0700 directory) and a detached child in a
// new session, neither of which has a small correct Windows equivalent here. A
// Windows session serves in-process, exactly as without --daemon.
func daemonSupported() bool { return false }

// listenDaemon is never reached on Windows (daemonSupported is false); it exists
// so the daemon code compiles.
func listenDaemon(string) (net.Listener, error) { return nil, errDaemonUnsupported }

// dialDaemon is never reached on Windows; it reports no daemon so a caller falls
// back in-process.
func dialDaemon(string) (net.Conn, error) { return nil, errDaemonMissing }

// ensureDaemonDir is never reached on Windows; it refuses so no directory is
// prepared for a daemon that cannot run.
func ensureDaemonDir(string) error { return errDaemonUnsupported }
