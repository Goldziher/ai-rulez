package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// The command line starts the server with a context that carries the policy
// (mcp.go), and the handlers read it from the request context. This runs a real
// client against that server over an in-process transport, so a change in how the
// SDK derives the request context from the one given to Run fails here.
func TestMCPPolicyReachesHandlersOverARealTransport(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithTimeout(config.WithPolicyContext(context.Background(), loosening{}), 30*time.Second)
	t.Cleanup(cancel)
	serverT, clientT := sdkmcp.NewInMemoryTransports()
	srv := NewServer("test", WithAnyDirectory())
	go func() { _ = srv.GetMCPServer().Run(ctx, serverT) }()
	session, err := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, clientT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	dir := telemetryProject(t)

	// Act
	generated, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "generate_outputs", Arguments: map[string]any{"working_directory": dir}})
	require.NoError(t, err)
	validated, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "validate_config", Arguments: map[string]any{"working_directory": dir}})
	require.NoError(t, err)

	// Assert
	assert.True(t, generated.IsError, "generate refuses a configuration that loosens the policy")
	assert.True(t, validated.IsError, "a configuration that loosens the policy is an error result, never success")
	require.NotEmpty(t, validated.Content)
	text, ok := validated.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))
	assert.Equal(t, false, payload["valid"])
	assert.Contains(t, payload["error"], "loosens the organization policy")
}
