package mcp

import (
	"context"
	"sync"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// JSON-RPC method names for the MCP initialization handshake. The SDK keeps its
// own copies unexported, so they are redeclared here rather than inlined.
const (
	methodInitialize        = "initialize"
	notificationInitialized = "notifications/initialized"
)

// sessionInitState is the handshake state the middleware tracks per session.
type sessionInitState struct {
	result      *sdkmcp.InitializeResult
	initialized bool
}

// initTracker holds the per-session handshake state of tolerantInitializeMiddleware.
// An entry lives until its session ends, so a long-lived process serving many
// sessions does not accumulate state or pin finished ServerSessions.
type initTracker struct {
	// server lists the live sessions; nil disables the sweep (unit tests that
	// drive the middleware without a server).
	server   *sdkmcp.Server
	mu       sync.Mutex
	sessions map[*sdkmcp.ServerSession]*sessionInitState
}

func newInitTracker(server *sdkmcp.Server) *initTracker {
	return &initTracker{server: server, sessions: make(map[*sdkmcp.ServerSession]*sessionInitState)}
}

// size is the number of sessions currently tracked.
func (t *initTracker) size() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.sessions)
}

// state returns the tracked state for the request's session, creating it on
// first use. Creating one first drops the state of sessions that have ended: the
// SDK has no close hook, but it removes a session from Sessions() when its
// connection closes, and a session is listed from before its first message.
// Callers must hold t.mu.
func (t *initTracker) state(request sdkmcp.Request) *sessionInitState {
	session, _ := request.GetSession().(*sdkmcp.ServerSession) //nolint:errcheck // a nil session is a valid key
	tracked, ok := t.sessions[session]
	if !ok {
		t.sweep()
		tracked = &sessionInitState{}
		t.sessions[session] = tracked
	}
	return tracked
}

// sweep forgets sessions the server no longer lists.
func (t *initTracker) sweep() {
	if t.server == nil {
		return
	}
	live := make(map[*sdkmcp.ServerSession]bool)
	for session := range t.server.Sessions() {
		live[session] = true
	}
	for session := range t.sessions {
		if session != nil && !live[session] {
			delete(t.sessions, session)
		}
	}
}

// tolerantInitializeMiddleware makes the server tolerate a repeated
// initialization handshake on an already-established session.
//
// The SDK treats initialization as a one-shot state machine: once
// ServerSession.state.InitializeParams is set, a second `initialize` fails with
// `duplicate "initialize" received` and a second `notifications/initialized`
// fails likewise, wedging the session for good. That breaks every MCP host that
// retries or reconnects over a long-lived stdio process — Claude Code's client,
// or an mcpm/fastmcp bridge shared between consumers. The SDK exposes no reset
// path, so the leniency the spec expects has to live here. See issue #158.
//
// Receiving middleware wraps the terminal method handler, so short-circuiting
// here means the duplicate never reaches the SDK and the session state stays
// untouched.
func tolerantInitializeMiddleware(server *sdkmcp.Server) sdkmcp.Middleware {
	return newInitTracker(server).middleware()
}

func (t *initTracker) middleware() sdkmcp.Middleware {
	return func(next sdkmcp.MethodHandler) sdkmcp.MethodHandler {
		return func(ctx context.Context, method string, request sdkmcp.Request) (sdkmcp.Result, error) {
			switch method {
			case methodInitialize:
				t.mu.Lock()
				cached := t.state(request).result
				t.mu.Unlock()

				// A repeat gets the result of the original negotiation. The SDK's
				// version negotiation is unexported, so re-negotiating a different
				// protocolVersion is not possible; replaying the established result
				// is the lenient behavior and keeps the session coherent.
				if cached != nil {
					return cached, nil
				}

				result, err := next(ctx, method, request)
				if err != nil {
					return nil, err
				}
				if initializeResult, ok := result.(*sdkmcp.InitializeResult); ok {
					t.mu.Lock()
					t.state(request).result = initializeResult
					t.mu.Unlock()
				}
				return result, nil

			case notificationInitialized:
				t.mu.Lock()
				seen := t.state(request).initialized
				t.mu.Unlock()

				if seen {
					return nil, nil
				}

				result, err := next(ctx, method, request)
				if err != nil {
					return nil, err
				}
				// Only a notification the SDK accepted establishes the session, so a
				// rejected one (e.g. arriving before `initialize`) must not suppress
				// the real notification that follows.
				t.mu.Lock()
				t.state(request).initialized = true
				t.mu.Unlock()
				return result, nil

			default:
				return next(ctx, method, request)
			}
		}
	}
}
