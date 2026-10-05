package presets

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func opencodeHooksConfig() *config.Config {
	return &config.Config{Name: "test", Hooks: []config.HookGroup{
		{Event: "PreToolUse", Matcher: "Bash", Hooks: []config.HookAction{{Command: "echo guard"}}},
	}}
}

func TestOpencodePresetGenerator_HooksPlugin(t *testing.T) {
	tests := []struct {
		name       string
		cfg        *config.Config
		wantPlugin bool
	}{
		{"hooks declared", opencodeHooksConfig(), true},
		{"no hooks", &config.Config{Name: "test"}, false},
		{"group targets another harness", &config.Config{Name: "test", Hooks: []config.HookGroup{
			{Event: "Stop", Targets: []string{config.HarnessCursor}, Hooks: []config.HookAction{{Command: "x"}}},
		}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			g := &OpencodePresetGenerator{}
			baseDir := t.TempDir()

			// Act
			outputs, err := g.Generate(&config.ContentTree{}, baseDir, tt.cfg)

			// Assert
			require.NoError(t, err)
			var plugin *config.OutputFile
			for i := range outputs {
				if outputs[i].Path == filepath.Join(baseDir, ".opencode", "plugins", "ai-rulez-hooks.js") {
					plugin = &outputs[i]
				}
			}
			if !tt.wantPlugin {
				assert.Nil(t, plugin)
				return
			}
			require.NotNil(t, plugin)
			assert.Contains(t, plugin.Content, `"command": "echo guard"`)
			assert.Contains(t, plugin.Content, "export default {")
			assert.False(t, plugin.PartiallyOwned, "the plugin is owned wholly by ai-rulez")
		})
	}
}

func TestOpencodePresetGenerator_HooksPluginUserScope(t *testing.T) {
	g := &OpencodePresetGenerator{}
	paths := g.GlobalOutputPaths("/home/u", func(string) string { return "" })

	assert.Equal(t, filepath.FromSlash("/home/u/.config/opencode/plugins/ai-rulez-hooks.js"),
		paths.Sidecars[".opencode/plugins/ai-rulez-hooks.js"])
}
