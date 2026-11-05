package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func codexRepo(t *testing.T, exposureTable string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		".ai-rulez/config.toml": "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\n\n[exposure]\n" + exposureTable,
		"AGENTS.md":             strings.Repeat("Codex instruction line.\n", 100),
		"CLAUDE.md":             "short\n",
		".mcp.json":             `{"mcpServers": {"one": {}, "two": {}}}`,
		".codex/config.toml":    "[mcp_servers.cx]\ncommand = \"c\"\n",
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}
	return root
}

func TestRunExposureReport_CodexUsesItsOwnBudget(t *testing.T) {
	// 100 lines of 24 bytes is about 616 tokens: over 500, under the claude budget of 5000.
	root := codexRepo(t, "start_budget = 5000\ncodex_start_budget = 500\n")

	var out bytes.Buffer
	err := runExposureReport(t.Context(), &out, &exposureFlags{harness: "codex", root: root})

	require.Error(t, err)
	assert.Equal(t, exitFindings, exitCodeOf(t, err))
	assert.Contains(t, out.String(), "[exposure] codex_start_budget 500")
	assert.Contains(t, out.String(), "agents_md")
	assert.NotContains(t, out.String(), "claude_md")

	out.Reset()
	require.NoError(t, runExposureReport(t.Context(), &out, &exposureFlags{harness: "claude", root: root}))
	assert.Contains(t, out.String(), "[exposure] start_budget 5000")
}

func TestRunExposureReport_HarnessFollowsTheConfigUnlessTheFlagIsGiven(t *testing.T) {
	root := codexRepo(t, "harness = \"codex\"\n")

	var out bytes.Buffer
	require.NoError(t, runExposureReport(t.Context(), &out, &exposureFlags{root: root}))
	assert.Contains(t, out.String(), "agents_md")

	out.Reset()
	require.NoError(t, runExposureReport(t.Context(), &out, &exposureFlags{harness: "claude", root: root}))
	assert.Contains(t, out.String(), "claude_md")
	assert.NotContains(t, out.String(), "agents_md")
}

func TestRunExposureInventory_StaysOnTheClaudeInventoryWhateverTheConfigHarness(t *testing.T) {
	root := codexRepo(t, "harness = \"codex\"\n")

	var out bytes.Buffer
	require.NoError(t, runExposureInventory(t.Context(), &out, &exposureFlags{root: root}))

	assert.Contains(t, out.String(), "claude_md")
	assert.NotContains(t, out.String(), "agents_md")
}

func TestRunExposureReport_PluginsAreRejectedUnderCodex(t *testing.T) {
	root := codexRepo(t, "")

	for _, plugins := range []string{"enabled", "all"} {
		var out bytes.Buffer
		err := runExposureReport(t.Context(), &out, &exposureFlags{harness: "codex", plugins: plugins, root: root})

		require.Error(t, err, plugins)
		assert.Contains(t, err.Error(), "--plugins")
		assert.Contains(t, err.Error(), "codex")
		assert.Empty(t, out.String())
	}

	var out bytes.Buffer
	require.NoError(t, runExposureReport(t.Context(), &out, &exposureFlags{harness: "codex", plugins: "none", root: root}))
	require.NoError(t, runExposureReport(t.Context(), &out, &exposureFlags{harness: "claude", plugins: "all", root: root}))

	cfgRoot := codexRepo(t, "harness = \"codex\"\n")
	err := runExposureReport(t.Context(), &out, &exposureFlags{plugins: "enabled", root: cfgRoot})
	require.Error(t, err, "the harness from [exposure] counts too")
}
