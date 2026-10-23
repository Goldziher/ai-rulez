package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/samber/oops"
)

// ServeViaDaemon proxies in and out to the shared daemon for the setup, starting
// one through opts.StartDaemon when none is listening. handled is false when the
// caller must serve in-process: the platform has no daemon, the socket is
// missing, unsafe or broken, or no daemon could be started. None of those is an
// error to report — the caller falls back to today's in-process server, so a
// missing or broken daemon never fails a session. A proxied session ends when
// either side closes, which is an ordinary end of stream, so err is nil then
// too.
func ServeViaDaemon(ctx context.Context, st *ServeSetup, in io.Reader, out io.Writer, opts ProxyOptions) (handled bool, err error) {
	log := st.logger()
	if !daemonSupported() {
		log.Warn("The shared MCP daemon is not supported on this platform; serving skills in-process")
		return false, nil
	}
	path := opts.SocketPath
	if path == "" {
		derived, derr := st.daemonSocketPath()
		if derr != nil {
			log.Warn("MCP daemon: cannot place a socket, so this session serves skills in-process and shares nothing", "error", derr.Error())
			return false, nil
		}
		path = derived
	}
	conn, derr := dialDaemon(path)
	switch {
	case derr == nil:
	case errors.Is(derr, errDaemonUnsafe):
		log.Warn("MCP daemon: refusing an unsafe socket; serving skills in-process", "error", derr.Error())
		return false, nil
	default:
		conn, derr = startAndDial(ctx, path, opts)
		if derr != nil {
			log.Debug("MCP daemon: no daemon could be reached; serving skills in-process", "error", derr.Error())
			return false, nil
		}
	}
	defer conn.Close() //nolint:errcheck // the proxy owns the connection
	reader, herr := daemonHelloExchange(conn, st)
	if herr != nil {
		log.Debug("MCP daemon: the handshake failed; serving skills in-process", "error", herr.Error())
		return false, nil
	}
	log.Debug("MCP daemon: proxying the session", "socket", path)
	pumpDaemon(ctx, conn, reader, in, out)
	return true, nil
}

// startAndDial starts a daemon and waits for it to listen. A nil StartDaemon
// means the caller never starts one, so this reports a missing daemon at once.
func startAndDial(ctx context.Context, path string, opts ProxyOptions) (net.Conn, error) {
	if opts.StartDaemon == nil {
		return nil, errDaemonMissing
	}
	if err := opts.StartDaemon(path); err != nil {
		return nil, oops.Wrapf(err, "start the shared daemon")
	}
	wait := opts.StartWait
	if wait <= 0 {
		wait = daemonStartWait
	}
	clock := ambient.Clock(nil)
	deadline := clock.Now().Add(wait)
	var last error
	for clock.Now().Before(deadline) {
		if ctx.Err() != nil {
			return nil, ctx.Err() //nolint:wrapcheck // the caller falls back either way
		}
		conn, err := dialDaemon(path)
		if err == nil {
			return conn, nil
		}
		if errors.Is(err, errDaemonUnsafe) {
			return nil, err
		}
		last = err
		time.Sleep(25 * time.Millisecond)
	}
	if last == nil {
		last = errDaemonMissing
	}
	return nil, oops.Wrapf(last, "wait for the shared daemon to listen")
}

// daemonHelloExchange sends the hello and checks the reply. It returns the
// buffered reader over the connection, so a byte the daemon sent after its reply
// is not lost when the proxy starts copying.
func daemonHelloExchange(conn net.Conn, st *ServeSetup) (*bufio.Reader, error) {
	if err := conn.SetDeadline(ambient.Clock(nil).Now().Add(daemonHandshakeTimeout)); err != nil {
		return nil, oops.Wrapf(err, "bound the daemon handshake")
	}
	data, err := json.Marshal(daemonHello{Protocol: daemonProtocol, Version: st.Version, View: st.ViewKey()})
	if err != nil {
		return nil, oops.Wrapf(err, "encode the daemon hello")
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return nil, oops.Wrapf(err, "send the daemon hello")
	}
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, oops.Wrapf(err, "read the daemon reply")
	}
	var resp daemonHelloResp
	if err := json.Unmarshal(bytes.TrimSpace(line), &resp); err != nil {
		return nil, oops.Wrapf(err, "parse the daemon reply")
	}
	if !resp.OK {
		return nil, oops.Errorf("the daemon refused the session: %s", resp.Error)
	}
	if resp.Version != st.Version {
		return nil, oops.Errorf("the daemon serves version %s, not %s", resp.Version, st.Version)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return nil, oops.Wrapf(err, "clear the daemon handshake deadline")
	}
	return reader, nil
}
