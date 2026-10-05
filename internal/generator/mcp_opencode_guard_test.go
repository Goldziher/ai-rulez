package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// opencode.json carries resolved MCP env like .mcp.json does, so the secret
// guard must refuse to write it when it is not gitignored, even when the
// always-generated .mcp.json is.
func TestGenerator_MCPEnv_GuardsOpencodeJSON(t *testing.T) {
	tempDir := t.TempDir()
	aiRulezDir := filepath.Join(tempDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(aiRulezDir, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(aiRulezDir, "config.toml"), []byte(`version = "4.0"
name = "opencode-guard"
presets = ["opencode"]
gitignore = false

[[mcp_servers]]
name = "grafana"
command = "uvx"
env = { GRAFANA_SERVICE_ACCOUNT_TOKEN = "literal-token" }
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte(".mcp.json\n.opencode/\n"), 0o644))
	cfg, err := config.LoadConfig(context.Background(), tempDir)
	require.NoError(t, err)

	err = NewGenerator(cfg).Generate("default")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "contains secrets but is not gitignored")
	assert.NoFileExists(t, filepath.Join(tempDir, "opencode.json"))
}
