package providers_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
)

func hookPluginConfig() *config.Config {
	return &config.Config{Name: "test", Hooks: []config.HookGroup{
		{Event: "PreToolUse", Matcher: "Bash", Hooks: []config.HookAction{{Command: "echo guard"}}},
		{Event: "Stop", Hooks: []config.HookAction{{Command: "echo done"}}},
	}}
}

func TestHookPluginSidecar_BuiltinSpecs(t *testing.T) {
	tests := []struct {
		preset, path, userPath, contains string
	}{
		{"kilo", ".kilo/plugins/ai-rulez-hooks.js", ".config/kilo/plugins/ai-rulez-hooks.js", "export default {"},
		{"mimocode", ".mimocode/plugins/ai-rulez-hooks.js", ".config/mimocode/plugins/ai-rulez-hooks.js", "export default {"},
		{"pi", ".pi/extensions/ai-rulez-hooks.ts", ".pi/agent/extensions/ai-rulez-hooks.ts", "pi.on(\"tool_call\""},
		{"amp", ".amp/plugins/ai-rulez-hooks.ts", ".config/amp/plugins/ai-rulez-hooks.ts", "amp.on(\"tool.call\""},
	}
	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			// Arrange
			gen, err := providers.LoadBuiltin(tt.preset)
			require.NoError(t, err)
			baseDir := t.TempDir()

			// Act
			outputs, err := gen.Generate(&config.ContentTree{}, baseDir, hookPluginConfig())

			// Assert
			require.NoError(t, err)
			var found *config.OutputFile
			for i := range outputs {
				if outputs[i].Path == filepath.Join(baseDir, filepath.FromSlash(tt.path)) {
					found = &outputs[i]
				}
			}
			require.NotNil(t, found, "the plugin module is generated")
			assert.Contains(t, found.Content, tt.contains)
			assert.Contains(t, found.Content, `"command": "echo guard"`)
			assert.False(t, found.PartiallyOwned)
			assert.Equal(t, absSlash("/home/u/"+tt.userPath),
				gen.GlobalOutputPaths(absSlash("/home/u"), func(string) string { return "" }).Sidecars[tt.path])

			// Without hooks there is no module.
			outputs, err = gen.Generate(&config.ContentTree{}, t.TempDir(), &config.Config{Name: "test"})
			require.NoError(t, err)
			for _, out := range outputs {
				assert.NotEqual(t, filepath.Base(tt.path), filepath.Base(out.Path))
			}
		})
	}
}

func TestHookPluginSidecar_Validation(t *testing.T) {
	const base = "name = \"tool\"\n[[sidecars]]\nkind = \"hook_plugin\"\npath = \"p/hooks.js\"\n"
	tests := []struct {
		name, extra, wantErr string
	}{
		{"valid", "flavor = \"opencode-v1\"\n", ""},
		{"flavor required", "", "flavor"},
		{"unknown flavor", "flavor = \"vim\"\n", "unknown flavor"},
		{"format is for generic kinds", "flavor = \"pi\"\nformat = \"json\"\n", "only valid on the generic kinds"},
		{"flavor on another kind", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := base + tt.extra
			if tt.name == "flavor on another kind" {
				body = "name = \"tool\"\n[[sidecars]]\nkind = \"mcp_json\"\npath = \"p.json\"\nflavor = \"pi\"\n"
				tt.wantErr = "flavor is only valid"
			}

			_, err := providers.LoadProviderSpec([]byte(body), "x.toml", providers.FormatTOML)

			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestHookPluginSidecar_IsNotAMergedDocument(t *testing.T) {
	assert.False(t, providers.SidecarIsMergedDocument(providers.SidecarHookPlugin))
	for _, doc := range providers.MergedSidecarDocs() {
		assert.NotContains(t, doc.Path, "ai-rulez-hooks")
	}
}

func TestHookPluginSidecar_LeavesAHandWrittenModuleAlone(t *testing.T) {
	gen, err := providers.LoadBuiltin("pi")
	require.NoError(t, err)
	baseDir := t.TempDir()
	module := filepath.Join(baseDir, ".pi", "extensions", "ai-rulez-hooks.ts")
	require.NoError(t, os.MkdirAll(filepath.Dir(module), 0o755))
	require.NoError(t, os.WriteFile(module, []byte("export default () => {}\n"), 0o644))

	outputs, err := gen.Generate(&config.ContentTree{}, baseDir, hookPluginConfig())

	require.NoError(t, err)
	for _, out := range outputs {
		assert.NotEqual(t, module, out.Path, "a module the consumer wrote is not overwritten")
	}
}
