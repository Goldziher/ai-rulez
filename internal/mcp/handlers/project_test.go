package handlers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resultPayload extracts the JSON-encoded payload from a ToolSuccess result.
func resultPayload(t *testing.T, res *sdkmcp.CallToolResult) map[string]interface{} {
	t.Helper()
	require.NotNil(t, res)
	require.NotEmpty(t, res.Content)
	tc, ok := res.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok, "expected TextContent, got %T", res.Content[0])
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(tc.Text), &out))
	return out
}

// writeMinimalConfig writes a minimal valid .ai-rulez/config.yaml inside dir.
func writeMinimalConfig(t *testing.T, dir string) {
	t.Helper()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	body := "version: \"4.0\"\nname: test\npresets:\n  - claude\n"
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(body), 0o644))
}

func newRequestWithArgs(args map[string]any) *ToolRequest {
	return NewToolRequest(nil, args)
}

func TestUpdateConfigHandler_SetsDefaultEffort(t *testing.T) {
	dir := t.TempDir()
	writeMinimalConfig(t, dir)

	req := newRequestWithArgs(map[string]any{
		"working_directory": dir,
		"default_effort":    "high",
	})

	res, err := UpdateConfigHandler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.IsError, "update should succeed")

	body, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(body), "defaults:")
	assert.Contains(t, string(body), "effort: high")
}

func TestUpdateConfigHandler_RejectsInvalidEffort(t *testing.T) {
	dir := t.TempDir()
	writeMinimalConfig(t, dir)

	req := newRequestWithArgs(map[string]any{
		"working_directory": dir,
		"default_effort":    "extreme",
	})

	res, err := UpdateConfigHandler(context.Background(), req)
	// ToolError surfaces an error tool result rather than returning a Go error.
	if err == nil {
		require.NotNil(t, res)
		assert.True(t, res.IsError, "expected validation failure to produce an error tool result")
	}

	body, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.yaml"))
	require.NoError(t, err)
	assert.NotContains(t, string(body), "extreme", "invalid effort must not be persisted")
}

func TestUpdateConfigHandler_ClearsDefaultEffortOnEmpty(t *testing.T) {
	dir := t.TempDir()
	writeMinimalConfig(t, dir)

	// First, set a default effort.
	setReq := newRequestWithArgs(map[string]any{
		"working_directory": dir,
		"default_effort":    "medium",
	})
	_, err := UpdateConfigHandler(context.Background(), setReq)
	require.NoError(t, err)

	// Then clear it by passing empty string.
	clearReq := newRequestWithArgs(map[string]any{
		"working_directory": dir,
		"default_effort":    "",
	})
	_, err = UpdateConfigHandler(context.Background(), clearReq)
	require.NoError(t, err)

	body, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.yaml"))
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(body), "effort:"),
		"effort key should be removed when cleared; got config:\n%s", string(body))
}

func TestReadConfigHandler_SurfacesDefaultEffort(t *testing.T) {
	dir := t.TempDir()
	writeMinimalConfig(t, dir)

	// Set default_effort first via UpdateConfigHandler so we exercise the round-trip.
	upd := newRequestWithArgs(map[string]any{
		"working_directory": dir,
		"default_effort":    "xhigh",
	})
	_, err := UpdateConfigHandler(context.Background(), upd)
	require.NoError(t, err)

	read := newRequestWithArgs(map[string]any{
		"working_directory": dir,
	})
	res, err := ReadConfigHandler(context.Background(), read)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.IsError)

	payload := resultPayload(t, res)
	assert.Equal(t, "xhigh", payload["default_effort"])
}

func TestReadConfigHandler_AlwaysEmitsDefaultEffortKey(t *testing.T) {
	dir := t.TempDir()
	writeMinimalConfig(t, dir)

	read := newRequestWithArgs(map[string]any{
		"working_directory": dir,
	})
	res, err := ReadConfigHandler(context.Background(), read)
	require.NoError(t, err)
	require.NotNil(t, res)

	payload := resultPayload(t, res)
	val, present := payload["default_effort"]
	assert.True(t, present, "default_effort key must always be in the response so clients can detect 'not set' vs 'absent'")
	assert.Equal(t, "", val, "default_effort should be empty string when nothing is configured")
}
