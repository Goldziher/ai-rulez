package commands

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunExposureReport_McpSchemasLineItem(t *testing.T) {
	root := codexRepo(t, "")

	var off bytes.Buffer
	require.NoError(t, runExposureReport(t.Context(), &off, &exposureFlags{harness: "claude", root: root}))
	assert.NotContains(t, off.String(), "mcp tool schemas")

	var on bytes.Buffer
	require.NoError(t, runExposureReport(t.Context(), &on, &exposureFlags{harness: "claude", root: root, mcpServerTokens: 250}))
	assert.Contains(t, on.String(), "mcp tool schemas (estimated): 2 servers x 250 = 500 tokens")

	var codex bytes.Buffer
	require.NoError(t, runExposureReport(t.Context(), &codex, &exposureFlags{harness: "codex", root: root, mcpServerTokens: 250}))
	assert.Contains(t, codex.String(), "mcp tool schemas (estimated): 1 server x 250 = 250 tokens")

	var js bytes.Buffer
	require.NoError(t, runExposureReport(t.Context(), &js, &exposureFlags{harness: "claude", root: root, mcpServerTokens: 250, asJSON: true}))
	var doc struct {
		Harness    string `json:"harness"`
		McpSchemas struct {
			Servers, TokensPerServer, Tokens int
		} `json:"mcp_schemas"`
	}
	require.NoError(t, json.Unmarshal(js.Bytes(), &doc))
	assert.Equal(t, 2, doc.McpSchemas.Servers)
	assert.Equal(t, 500, doc.McpSchemas.Tokens)
}

func TestRunExposureReport_McpServerTokensFromConfig(t *testing.T) {
	root := codexRepo(t, "mcp_server_tokens = 100\n")

	var out bytes.Buffer
	require.NoError(t, runExposureReport(t.Context(), &out, &exposureFlags{harness: "claude", root: root}))

	assert.Contains(t, out.String(), "2 servers x 100 = 200 tokens")
}
