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

// .xum/mcp.jsonc carries resolved MCP env and headers, so the secret guard must
// refuse to write it while it is not gitignored.
func TestGenerator_MCPSecrets_GuardsXumMCPConfig(t *testing.T) {
	tempDir := t.TempDir()
	aiRulezDir := filepath.Join(tempDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(aiRulezDir, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(aiRulezDir, "config.toml"), []byte(`version = "4.0"
name = "xum-guard"
presets = ["xum"]
gitignore = false

[[mcp_servers]]
name = "api"
transport = "http"
url = "https://example.com/mcp"
headers = { Authorization = "Bearer literal-token" }
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte(".mcp.json\n"), 0o644))
	cfg, err := config.LoadConfig(context.Background(), tempDir)
	require.NoError(t, err)

	err = NewGenerator(cfg).Generate("default")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "contains secrets but is not gitignored")
	assert.NoFileExists(t, filepath.Join(tempDir, ".xum", "mcp.jsonc"))
}
