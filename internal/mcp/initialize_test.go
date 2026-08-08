package mcp

import (
	"context"
	"errors"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingHandler records how many times the wrapped terminal handler ran, so
// the tests can assert that duplicate initialize traffic never reaches the SDK
// (where it would be rejected with `duplicate "initialize" received`).
type countingHandler struct {
	calls   int
	methods []string
	result  sdkmcp.Result
	err     error
}

func (h *countingHandler) handle(_ context.Context, method string, _ sdkmcp.Request) (sdkmcp.Result, error) {
	h.calls++
	h.methods = append(h.methods, method)
	return h.result, h.err
}

func initializeRequest() sdkmcp.Request {
	return &sdkmcp.ServerRequest[*sdkmcp.InitializeParams]{
		Params: &sdkmcp.InitializeParams{
			ProtocolVersion: "2025-06-18",
			ClientInfo:      &sdkmcp.Implementation{Name: "test-client", Version: "0.1"},
		},
	}
}

func initializedRequest() sdkmcp.Request {
	return &sdkmcp.ServerRequest[*sdkmcp.InitializedParams]{Params: &sdkmcp.InitializedParams{}}
}

// TestTolerantInitializeMiddleware covers issue #158: MCP hosts that retry or
// reconnect re-send `initialize` on an established stdio session, which the SDK
// rejects outright. The middleware must absorb the repeat instead.
func TestTolerantInitializeMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("should_return_cached_result_when_initialize_repeats", func(t *testing.T) {
		t.Parallel()

		want := &sdkmcp.InitializeResult{
			ProtocolVersion: "2025-06-18",
			ServerInfo:      &sdkmcp.Implementation{Name: "ai-rulez", Version: "4.11.2"},
		}
		next := &countingHandler{result: want}
		handler := tolerantInitializeMiddleware()(next.handle)

		first, err := handler(context.Background(), methodInitialize, initializeRequest())
		require.NoError(t, err)
		assert.Same(t, want, first)

		second, err := handler(context.Background(), methodInitialize, initializeRequest())
		require.NoError(t, err, "a repeated initialize must not error")
		assert.Same(t, want, second, "the repeat must get the originally negotiated result")
		assert.Equal(t, 1, next.calls, "the duplicate initialize must not reach the SDK handler")
	})

	t.Run("should_swallow_duplicate_initialized_notification", func(t *testing.T) {
		t.Parallel()

		next := &countingHandler{}
		handler := tolerantInitializeMiddleware()(next.handle)

		_, err := handler(context.Background(), notificationInitialized, initializedRequest())
		require.NoError(t, err)

		result, err := handler(context.Background(), notificationInitialized, initializedRequest())
		require.NoError(t, err, "a repeated initialized notification must not error")
		assert.Nil(t, result)
		assert.Equal(t, 1, next.calls, "the duplicate notification must not reach the SDK handler")
	})

	t.Run("should_pass_through_other_methods", func(t *testing.T) {
		t.Parallel()

		next := &countingHandler{}
		handler := tolerantInitializeMiddleware()(next.handle)

		for range 3 {
			_, err := handler(context.Background(), "tools/list", initializeRequest())
			require.NoError(t, err)
		}

		assert.Equal(t, 3, next.calls, "unrelated methods must always reach the SDK handler")
		assert.Equal(t, []string{"tools/list", "tools/list", "tools/list"}, next.methods)
	})

	t.Run("should_not_cache_when_initialize_fails", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("bad protocol version")
		next := &countingHandler{err: wantErr}
		handler := tolerantInitializeMiddleware()(next.handle)

		_, err := handler(context.Background(), methodInitialize, initializeRequest())
		require.ErrorIs(t, err, wantErr)

		_, err = handler(context.Background(), methodInitialize, initializeRequest())
		require.ErrorIs(t, err, wantErr)
		assert.Equal(t, 2, next.calls, "a failed initialize must not be cached as an established session")
	})

	t.Run("should_track_sessions_independently", func(t *testing.T) {
		t.Parallel()

		next := &countingHandler{result: &sdkmcp.InitializeResult{ProtocolVersion: "2025-06-18"}}
		handler := tolerantInitializeMiddleware()(next.handle)

		sessionA := &sdkmcp.ServerRequest[*sdkmcp.InitializeParams]{
			Session: &sdkmcp.ServerSession{},
			Params:  &sdkmcp.InitializeParams{ProtocolVersion: "2025-06-18"},
		}
		sessionB := &sdkmcp.ServerRequest[*sdkmcp.InitializeParams]{
			Session: &sdkmcp.ServerSession{},
			Params:  &sdkmcp.InitializeParams{ProtocolVersion: "2025-06-18"},
		}

		_, err := handler(context.Background(), methodInitialize, sessionA)
		require.NoError(t, err)
		_, err = handler(context.Background(), methodInitialize, sessionB)
		require.NoError(t, err)

		assert.Equal(t, 2, next.calls, "a second session must initialize on its own, not read another session's cache")
	})
}
