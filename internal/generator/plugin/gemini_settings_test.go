package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGeminiSettingsLoadedAndRendered(t *testing.T) {
	directory := t.TempDir()
	body := `version = "5.0"
name = "example"
[plugin]
name = "example"
version = "1.0.0"
description = "Example MCP extension."
runtimes = ["gemini"]
[[plugin.gemini.settings]]
name = "Provider key"
description = "Configure the provider used for requests."
env_var = "OPENAI_API_KEY"
sensitive = true
`
	require.NoError(t, os.Mkdir(filepath.Join(directory, ".ai-rulez"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, ".ai-rulez", "config.toml"), []byte(body), 0o600))
	cfg, err := config.LoadConfig(context.Background(), directory)
	require.NoError(t, err)
	manifest, err := BuildManifest(cfg, cfg.Content)
	require.NoError(t, err)
	output := render(t, manifest)
	doc := parseJSON(t, output["gemini-extension.json"])
	require.Equal(t, []any{map[string]any{"name": "Provider key", "description": "Configure the provider used for requests.", "envVar": "OPENAI_API_KEY", "sensitive": true}}, doc["settings"])
	require.NotContains(t, string(output["gemini-extension.json"]), "env_var")
	require.NotContains(t, string(output["gemini-extension.json"]), "default")
}
