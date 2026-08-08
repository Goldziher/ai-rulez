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
func tolerantInitializeMiddleware() sdkmcp.Middleware {
	var (
		mu       sync.Mutex
		sessions = make(map[*sdkmcp.ServerSession]*sessionInitState)
	)

	// state returns the tracked state for the request's session, creating it on
	// first use. Callers must hold mu.
	state := func(request sdkmcp.Request) *sessionInitState {
		session, _ := request.GetSession().(*sdkmcp.ServerSession) //nolint:errcheck // a nil session is a valid key
		tracked, ok := sessions[session]
		if !ok {
			tracked = &sessionInitState{}
			sessions[session] = tracked
		}
		return tracked
	}

	return func(next sdkmcp.MethodHandler) sdkmcp.MethodHandler {
		return func(ctx context.Context, method string, request sdkmcp.Request) (sdkmcp.Result, error) {
			switch method {
			case methodInitialize:
				mu.Lock()
				tracked := state(request)
				cached := tracked.result
				mu.Unlock()

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
					mu.Lock()
					state(request).result = initializeResult
					mu.Unlock()
				}
				return result, nil

			case notificationInitialized:
				mu.Lock()
				tracked := state(request)
				seen := tracked.initialized
				tracked.initialized = true
				mu.Unlock()

				if seen {
					return nil, nil
				}
				return next(ctx, method, request)

			default:
				return next(ctx, method, request)
			}
		}
	}
}
