package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func layerProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".ai-rulez")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	return root
}

const layerMain = `version = "5.0"
name = "proj"
presets = ["claude"]

[[mcp_servers]]
name = "base"
command = "echo"
`

func TestLayers_MergeInOrderAndLaterWins(t *testing.T) {
	root := layerProject(t, map[string]string{
		"config.toml": layerMain,
		"config.ai.toml": `
[[mcp_servers]]
name = "basemind"
command = "basemind"
args = ["serve"]

[[mcp_servers]]
name = "base"
command = "from-ai"
`,
		"config.custom.toml": `
[[mcp_servers]]
name = "base"
command = "from-custom"
`,
	})

	cfg, err := LoadConfig(context.Background(), root, WithLayers("ai", "custom"))
	require.NoError(t, err)
	assert.Equal(t, []string{"ai", "custom"}, cfg.ActiveLayers)
	byName := map[string]string{}
	for _, s := range cfg.MCPServersRaw {
		byName[s.Name] = s.Command
	}
	assert.Equal(t, map[string]string{"base": "from-custom", "basemind": "basemind"}, byName)

	swapped, err := LoadConfig(context.Background(), root, WithLayers("custom", "ai"))
	require.NoError(t, err)
	for _, s := range swapped.MCPServersRaw {
		if s.Name == "base" {
			assert.Equal(t, "from-ai", s.Command, "the layer named last wins")
		}
	}
}

func TestLayers_EnvSelectsLayersAndMissingFilesAreSkipped(t *testing.T) {
	root := layerProject(t, map[string]string{
		"config.toml":    layerMain,
		"config.ai.toml": "[[mcp_servers]]\nname = \"basemind\"\ncommand = \"basemind\"\n",
	})
	t.Setenv("AI_RULEZ_PROFILE", "core,ai,default")
	cfg, err := LoadConfig(context.Background(), root)
	require.NoError(t, err)
	assert.Equal(t, []string{"ai"}, cfg.ActiveLayers)
	assert.Len(t, cfg.MCPServersRaw, 2)

	t.Setenv("AI_RULEZ_LAYERS", "core")
	none, err := LoadConfig(context.Background(), root)
	require.NoError(t, err)
	assert.Empty(t, none.ActiveLayers, "AI_RULEZ_LAYERS wins over AI_RULEZ_PROFILE")
	assert.Len(t, none.MCPServersRaw, 1)
}

func TestLayers_LocalOverlayAppliesAfterLayers(t *testing.T) {
	root := layerProject(t, map[string]string{
		"config.toml":       layerMain,
		"config.ai.toml":    "[[mcp_servers]]\nname = \"base\"\ncommand = \"from-ai\"\n",
		"config.local.toml": "[[mcp_servers]]\nname = \"base\"\ncommand = \"from-local\"\n",
	})
	cfg, err := LoadConfig(context.Background(), root, WithLayers("ai"))
	require.NoError(t, err)
	require.NotNil(t, cfg.LocalOverlay)
	assert.Equal(t, []string{"ai"}, cfg.ActiveLayers)
	assert.Equal(t, "from-local", cfg.MCPServersRaw[0].Command)
}

func TestLayers_RejectBadNamesAndRefuseWrite(t *testing.T) {
	root := layerProject(t, map[string]string{"config.toml": layerMain, "config.ai.toml": ""})
	for _, name := range []string{"local", "../x", "a/b"} {
		_, err := LoadConfig(context.Background(), root, WithLayers(name))
		require.Error(t, err, name)
	}
	cfg, err := LoadConfig(context.Background(), root, WithLayers("ai"))
	require.NoError(t, err)
	_, err = MarshalTOML(cfg)
	require.Error(t, err, "a layered view is never written back")

	assert.Equal(t, []string{"ai"}, LayerFiles(filepath.Join(root, ".ai-rulez")))
}
