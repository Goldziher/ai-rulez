package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
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

func TestRunExposureReport_ExplicitZeroMcpServerTokensTurnsTheConfigOff(t *testing.T) {
	root := codexRepo(t, "mcp_server_tokens = 100\n")
	cmd := newExposureCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--root", root, "--harness", "claude", "--mcp-server-tokens", "0"})

	require.NoError(t, cmd.ExecuteContext(t.Context()))

	assert.NotContains(t, out.String(), "mcp tool schemas")
}

func TestRunExposureReport_McpSchemasAppearInTheByKindBreakdown(t *testing.T) {
	root := codexRepo(t, "")

	var text bytes.Buffer
	require.NoError(t, runExposureReport(t.Context(), &text, &exposureFlags{harness: "claude", root: root, mcpServerTokens: 250}))
	assert.Contains(t, text.String(), "  mcp_schemas: 500")

	var js bytes.Buffer
	require.NoError(t, runExposureReport(t.Context(), &js, &exposureFlags{harness: "claude", root: root, mcpServerTokens: 250, asJSON: true}))
	var doc struct {
		StartTokens       int            `json:"start_tokens"`
		StartTokensByKind map[string]int `json:"start_tokens_by_kind"`
	}
	require.NoError(t, json.Unmarshal(js.Bytes(), &doc))
	sum := 0
	for _, n := range doc.StartTokensByKind {
		sum += n
	}
	assert.Equal(t, doc.StartTokens, sum, "the by-kind breakdown sums to the start tokens")
	assert.Equal(t, 500, doc.StartTokensByKind["mcp_schemas"])

	var off bytes.Buffer
	require.NoError(t, runExposureReport(t.Context(), &off, &exposureFlags{harness: "claude", root: root}))
	assert.NotContains(t, off.String(), "mcp_schemas")
}

func TestRunExposureReport_DisabledCodexMcpServersAreNotCounted(t *testing.T) {
	root := codexRepo(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(root, ".codex", "config.toml"),
		[]byte("[mcp_servers.on]\ncommand = \"c\"\n\n[mcp_servers.off]\ncommand = \"c\"\nenabled = false\n"), 0o600))

	var out bytes.Buffer
	require.NoError(t, runExposureReport(t.Context(), &out, &exposureFlags{harness: "codex", root: root, mcpServerTokens: 100}))

	assert.Contains(t, out.String(), "1 server x 100 = 100 tokens")
}

func TestExposureLintSourceStartExposureCountsMcpSchemas(t *testing.T) {
	root := codexRepo(t, "")
	tokens := func(exp *config.ExposureConfig) int {
		src := exposureLintSource{ctx: t.Context(), cfg: &config.Config{BaseDir: root, Exposure: exp}, cache: &exposureCache{}}
		start := src.StartExposure(config.ExposureHarnessClaude)
		require.NotNil(t, start)
		return start.StartTokens
	}

	without := tokens(&config.ExposureConfig{StartBudget: 300})
	with := tokens(&config.ExposureConfig{StartBudget: 300, McpServerTokens: 250})

	assert.Equal(t, 500, with-without, "two .mcp.json servers at 250 tokens each, as `exposure` counts them")
}
