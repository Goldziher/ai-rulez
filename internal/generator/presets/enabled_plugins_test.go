package presets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func enabledPluginsConfig(baseDir string, plugins ...config.EnabledPlugin) *config.Config {
	return &config.Config{Name: "test", BaseDir: baseDir, EnabledPlugins: plugins}
}

func pluginOutputBySuffix(t *testing.T, outputs []config.OutputFile, suffix string) *config.OutputFile {
	t.Helper()
	for i := range outputs {
		if strings.HasSuffix(filepath.ToSlash(outputs[i].Path), suffix) {
			return &outputs[i]
		}
	}
	return nil
}

func TestCodexPresetGenerator_EnabledPlugins(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		plugins  []config.EnabledPlugin
		want     []string
		notWant  []string
	}{
		{
			name:    "fresh config.toml",
			plugins: []config.EnabledPlugin{{ID: "basemind@basemind", OpenCode: "basemind-opencode"}},
			want:    []string{"[plugins.\"basemind@basemind\"]", "enabled = true"},
			notWant: []string{"basemind-opencode"},
		},
		{
			name:     "user keys, comments and plugins survive",
			existing: "# my settings\nmodel = \"gpt-5\"\n\n[plugins.\"mine@theirs\"]\nenabled = false\n",
			plugins:  []config.EnabledPlugin{{ID: "basemind@basemind"}},
			want:     []string{"# my settings", "model = \"gpt-5\"", "[plugins.\"mine@theirs\"]", "enabled = false", "[plugins.\"basemind@basemind\"]"},
		},
		{
			name:    "several plugins",
			plugins: []config.EnabledPlugin{{ID: "a@m"}, {ID: "b@m"}},
			want:    []string{"[plugins.\"a@m\"]", "[plugins.\"b@m\"]"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			if tt.existing != "" {
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".codex", "config.toml"), []byte(tt.existing), 0o600))
			}
			g := &CodexPresetGenerator{}

			// Act
			outputs, err := g.Generate(&config.ContentTree{}, dir, enabledPluginsConfig(dir, tt.plugins...))

			// Assert
			require.NoError(t, err)
			found := pluginOutputBySuffix(t, outputs, ".codex/config.toml")
			require.NotNil(t, found, "expected .codex/config.toml to be emitted")
			for _, w := range tt.want {
				assert.Contains(t, found.Content, w)
			}
			for _, w := range tt.notWant {
				assert.NotContains(t, found.Content, w)
			}
			if tt.existing != "" {
				assert.True(t, found.PartiallyOwned, "a document with the user's keys is partially owned")
			}
		})
	}
}

func TestOpencodePresetGenerator_EnabledPlugins(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		plugins  []config.EnabledPlugin
		want     []any
		wantKeys []string
	}{
		{
			name:    "fresh opencode.json",
			plugins: []config.EnabledPlugin{{ID: "basemind@basemind", OpenCode: "basemind-opencode"}},
			want:    []any{"basemind-opencode"},
		},
		{
			name:     "user plugins and keys survive in order",
			existing: `{"model": "anthropic/claude", "plugin": ["theirs-plugin", "@scope/other@1.0.0"]}`,
			plugins:  []config.EnabledPlugin{{ID: "basemind@basemind", OpenCode: "basemind-opencode"}},
			want:     []any{"theirs-plugin", "@scope/other@1.0.0", "basemind-opencode"},
			wantKeys: []string{"model"},
		},
		{
			name:     "package the user already lists stays theirs and is not duplicated",
			existing: `{"plugin": ["basemind-opencode"]}`,
			plugins:  []config.EnabledPlugin{{ID: "basemind@basemind", OpenCode: "basemind-opencode"}},
			want:     []any{"basemind-opencode"},
		},
		{
			name:    "entries without an opencode package write no plugin key",
			plugins: []config.EnabledPlugin{{ID: "tool@official"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			path := filepath.Join(dir, "opencode.json")
			if tt.existing != "" {
				require.NoError(t, os.WriteFile(path, []byte(tt.existing), 0o600))
			}
			g := &OpencodePresetGenerator{}

			// Act
			result, err := g.renderMCPConfig(path, enabledPluginsConfig(dir, tt.plugins...))

			// Assert
			require.NoError(t, err)
			var doc map[string]any
			require.NoError(t, json.Unmarshal([]byte(result.Body), &doc))
			if tt.want == nil {
				assert.NotContains(t, doc, "plugin")
			} else {
				assert.Equal(t, tt.want, doc["plugin"])
			}
			for _, key := range tt.wantKeys {
				assert.Contains(t, doc, key)
			}
		})
	}
}
