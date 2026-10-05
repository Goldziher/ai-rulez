package providers_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
)

func hooksConfig(baseDir string) *config.Config {
	return &config.Config{
		Name: "t", BaseDir: baseDir,
		Hooks: []config.HookGroup{{
			Event: "PreToolUse", Matcher: "Bash", Matchers: map[string]string{"crush": "bash"},
			Hooks: []config.HookAction{{Command: "echo guard", Timeout: 10}},
		}},
		MCPServers: map[string]*config.MCPServer{"srv": {Name: "srv", Command: "npx", Args: []string{"-y", "srv"}}},
	}
}

func TestHooksSidecar_RendersTheDialectsDocument(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, spec, path, existing string
		want                       []string
		notWant                    []string
	}{
		{
			name:     "json hooks beside hand-written settings",
			spec:     "[[sidecars]]\nkind = \"hooks\"\ndialect = \"qwen\"\npath = \".qwen/settings.json\"\n",
			path:     ".qwen/settings.json",
			existing: `{"model": "mine", "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "mine"}]}]}}`,
			want:     []string{`"model": "mine"`, `"command": "mine"`, `"matcher": "Bash"`, `"command": "echo guard"`, `"timeout": 10`},
		},
		{
			name:     "toml flat list keeps the consumer's hook",
			spec:     "[[sidecars]]\nkind = \"hooks\"\ndialect = \"vibe\"\npath = \".vibe/hooks.toml\"\n",
			path:     ".vibe/hooks.toml",
			existing: "# mine\n[[hooks]]\nname = \"mine\"\ntype = \"pre_tool\"\nmatch = \"bash\"\ncommand = \"mine\"\n",
			want:     []string{"# mine", `name = "mine"`, `type = "pre_tool"`, `command = "echo guard"`},
			notWant:  []string{`match = "Bash"`},
		},
		{
			name:     "yaml hooks",
			spec:     "[[sidecars]]\nkind = \"hooks\"\ndialect = \"poolside\"\npath = \".poolside/settings.yaml\"\n",
			path:     ".poolside/settings.yaml",
			existing: "# mine\nmodel: x\n",
			want:     []string{"# mine", "model: x", "PreToolUse:", "command: echo guard"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			baseDir := writeFixture(t, tt.path, tt.existing)
			gen := loadSpec(t, "name = \"tool\"\n"+tt.spec)
			cfg := hooksConfig(baseDir)
			cfg.Hooks[0].Matchers = map[string]string{"poolside": "shell", "vibe": "bash"}

			// Act
			outputs, err := gen.Generate(&config.ContentTree{}, baseDir, cfg)

			// Assert
			require.NoError(t, err)
			out := requireFile(t, outputs, tt.path)
			assert.True(t, out.PartiallyOwned)
			assert.NotEmpty(t, out.MergeClaims)
			for _, want := range tt.want {
				assert.Contains(t, out.Content, want)
			}
			for _, not := range tt.notWant {
				assert.NotContains(t, out.Content, not)
			}
		})
	}
}

// TestHooksSidecar_SharesItsDocumentWithTheMCPSidecar checks that an mcp and a
// hooks sidecar of one document are merged together instead of the last one
// overwriting the first.
func TestHooksSidecar_SharesItsDocumentWithTheMCPSidecar(t *testing.T) {
	t.Parallel()

	// Arrange
	gen := loadSpec(t, `name = "tool"
[[sidecars]]
kind = "mcp"
dialect = "vscode"
key = ["mcp"]
path = "crush.json"

[[sidecars]]
kind = "hooks"
dialect = "crush"
path = "crush.json"
`)
	baseDir := writeFixture(t, "crush.json", `{"options": {"debug": true}}`)

	// Act
	outputs, err := gen.Generate(&config.ContentTree{}, baseDir, hooksConfig(baseDir))

	// Assert
	require.NoError(t, err)
	count := 0
	for _, o := range outputs {
		if filepath.Base(o.Path) == "crush.json" {
			count++
		}
	}
	assert.Equal(t, 1, count, "one document, one output")
	out := requireFile(t, outputs, "crush.json")
	for _, want := range []string{`"debug": true`, `"srv"`, `"PreToolUse"`, `"command": "echo guard"`} {
		assert.Contains(t, out.Content, want)
	}
}

func TestHooksSidecar_EmitsNothingWithoutApplicableHooks(t *testing.T) {
	t.Parallel()

	gen := loadSpec(t, "name = \"tool\"\n[[sidecars]]\nkind = \"hooks\"\ndialect = \"crush\"\npath = \"crush.json\"\n")
	baseDir := t.TempDir()

	t.Run("no hooks", func(t *testing.T) {
		t.Parallel()
		outputs, err := gen.Generate(&config.ContentTree{}, baseDir, &config.Config{Name: "t", BaseDir: baseDir})
		require.NoError(t, err)
		assert.False(t, hasOutputPathSuffix(outputs, "crush.json"))
	})
	t.Run("only events the harness lacks", func(t *testing.T) {
		t.Parallel()
		cfg := &config.Config{Name: "t", BaseDir: baseDir, Hooks: []config.HookGroup{{
			Event: "Stop", Hooks: []config.HookAction{{Command: "x"}},
		}}}
		outputs, err := gen.Generate(&config.ContentTree{}, baseDir, cfg)
		require.NoError(t, err)
		assert.False(t, hasOutputPathSuffix(outputs, "crush.json"))
	})
}

// TestHooksSidecar_OwnedFileDialectIsNotAMergedDocument checks that a hooks file
// ai-rulez writes whole (Copilot's) stays out of the merged-document registry, so
// a stale one is deleted rather than protected as the consumer's.
func TestHooksSidecar_OwnedFileDialectIsNotAMergedDocument(t *testing.T) {
	t.Parallel()

	assert.NotContains(t, providers.MergedSidecarPaths(), ".github/hooks/ai-rulez.json")
	assert.Contains(t, providers.MergedSidecarPaths(), ".qwen/settings.json")
}

// TestGenericSidecar_WithNoOwnedKeysLeavesTheUsersFileAlone checks that a hooks or
// permissions sidecar whose predicate holds but that owns nothing in this run (every
// hook group names an event the harness lacks) renders no output even when the user's
// file exists: it would otherwise be rewritten and registered as generated.
func TestGenericSidecar_WithNoOwnedKeysLeavesTheUsersFileAlone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, spec, path, existing string
	}{
		{
			name: "json hooks sidecar",
			spec: "[[sidecars]]\nkind = \"hooks\"\ndialect = \"crush\"\npath = \"crush.json\"\n",
			path: "crush.json", existing: "{\n  \"model\": \"x\"\n}\n",
		},
		{
			name: "toml hooks sidecar",
			spec: "[[sidecars]]\nkind = \"hooks\"\ndialect = \"vibe\"\npath = \"hooks.toml\"\n",
			path: "hooks.toml", existing: "# mine\nmodel = \"x\"\n",
		},
		{
			name: "yaml hooks sidecar",
			spec: "[[sidecars]]\nkind = \"hooks\"\ndialect = \"poolside\"\npath = \"settings.yaml\"\n",
			path: "settings.yaml", existing: "# mine\nmodel: x\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			baseDir := writeFixture(t, tt.path, tt.existing)
			gen := loadSpec(t, "name = \"tool\"\n"+tt.spec)
			cfg := &config.Config{Name: "t", BaseDir: baseDir, Hooks: []config.HookGroup{{
				Event: "FileChanged", Hooks: []config.HookAction{{Command: "x"}},
			}}}

			// Act
			outputs, err := gen.Generate(&config.ContentTree{}, baseDir, cfg)

			// Assert
			require.NoError(t, err)
			assert.False(t, hasOutputPathSuffix(outputs, tt.path))
		})
	}
}

func TestLoadProviderSpec_SidecarsSharingAPathMustAgree(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, body, wantErr string
	}{
		{
			name: "different global paths",
			body: "[[sidecars]]\nkind = \"hooks\"\ndialect = \"crush\"\npath = \"a.json\"\nglobal_path = \".t/a.json\"\n" +
				"[[sidecars]]\nkind = \"permissions\"\ndialect = \"qwen\"\npath = \"a.json\"\n",
			wantErr: "global_path",
		},
		{
			name: "different formats",
			body: "[[sidecars]]\nkind = \"hooks\"\ndialect = \"crush\"\npath = \"a.json\"\nformat = \"json\"\n" +
				"[[sidecars]]\nkind = \"permissions\"\ndialect = \"qwen\"\npath = \"a.json\"\nformat = \"toml\"\n",
			wantErr: "format",
		},
		{
			name: "agreeing sidecars",
			body: "[[sidecars]]\nkind = \"hooks\"\ndialect = \"crush\"\npath = \"a.json\"\nglobal_path = \".t/a.json\"\n" +
				"[[sidecars]]\nkind = \"permissions\"\ndialect = \"qwen\"\npath = \"a.json\"\nglobal_path = \".t/a.json\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := providers.LoadProviderSpec([]byte("name = \"tool\"\n"+tt.body), "spec.toml", providers.FormatAuto)

			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLegacyMergeClaims_CoverSpecDeclaredHooksAndPermissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rel  string
		want [][]string
	}{
		{name: "qwen settings", rel: ".qwen/settings.json", want: [][]string{{"hooks", "PreToolUse"}, {"permissions", "deny"}}},
		{name: "vibe hooks toml", rel: ".vibe/hooks.toml", want: [][]string{{"hooks"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			cfg := hooksConfig(t.TempDir())
			cfg.Hooks[0].Matchers = map[string]string{"vibe": "bash"}
			cfg.Permissions = &config.Permissions{Deny: []string{"Bash(rm -rf:*)"}}

			// Act
			claims := providers.LegacyMergeClaims(tt.rel, cfg)

			// Assert
			var paths [][]string
			for _, claim := range claims {
				paths = append(paths, claim.Path)
			}
			for _, want := range tt.want {
				assert.Contains(t, paths, want)
			}
		})
	}
}
