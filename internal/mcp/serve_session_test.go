package mcp

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A transport without a session id (stdio) still gets one id per connection, so
// usage lines carry a session and the budget is per connection.
func TestServeSession_StdioConnectionsGetTheirOwnSessionAndBudget(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var events []SessionTelemetry
	srv := NewSkillServerWith("test", loadCatalog(t), ServeOptions{BudgetBytes: 1500, Telemetry: func(e SessionTelemetry) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	}})
	first, second := connect(t, srv), connect(t, srv)
	args := map[string]any{"name": "pdf-processing", "path": "references/FORMS.md"}

	// Act
	r1 := call(t, first, "load_skill", args)
	r1again := call(t, first, "load_skill", args)
	r2 := call(t, second, "load_skill", args)

	// Assert
	require.False(t, r1.IsError)
	assert.True(t, r1again.IsError, "the same connection keeps one budget")
	assert.False(t, r2.IsError, "a new connection gets a new budget")
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, events, 2)
	assert.NotEmpty(t, events[0].Session)
	assert.NotEmpty(t, events[1].Session)
	assert.NotEqual(t, events[0].Session, events[1].Session)
}

// usedTotal sums the bytes charged to every connection of a server; a test with
// one connection reads that connection's usage.
func usedTotal(srv *Server) int {
	srv.serve.mu.Lock()
	defer srv.serve.mu.Unlock()
	total := 0
	for _, n := range srv.serve.used {
		total += n
	}
	return total
}
