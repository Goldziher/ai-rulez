package providers_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func permissionsConfig(dir string) *config.Config {
	return &config.Config{
		Name: "t", BaseDir: dir,
		Permissions: &config.Permissions{Allow: []string{"Bash(npm run test:*)"}, Deny: []string{"Bash(rm -rf:*)"}},
	}
}

func TestPermissionsSidecar_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sidecar string
		wantErr string
	}{
		{"unknown dialect", `dialect = "nope"`, "needs a permissions dialect"},
		{"missing dialect", ``, "needs a permissions dialect"},
		{"key is fixed by the dialect", "dialect = \"opencode\"\nkey = [\"permission\"]", "takes no key or elements"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			_, err := providers.LoadProviderSpec([]byte("name = \"t\"\n[[sidecars]]\nkind = \"permissions\"\npath = \"p.json\"\n"+tt.sidecar+"\n"),
				"spec.toml", providers.FormatAuto)

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestPermissionsSidecar_RendersAndSharesTheDocumentWithMCP(t *testing.T) {
	t.Parallel()

	gen := loadSpec(t, "name = \"tool\"\n"+
		"[[sidecars]]\nkind = \"mcp\"\npath = \"tool.jsonc\"\ndialect = \"opencode\"\n"+
		"[[sidecars]]\nkind = \"permissions\"\npath = \"tool.jsonc\"\ndialect = \"opencode\"\n")

	t.Run("permissions only", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		outputs, err := gen.Generate(&config.ContentTree{}, dir, permissionsConfig(dir))

		require.NoError(t, err)
		body, ok := findOutput(outputs, "tool.jsonc")
		require.True(t, ok)
		assert.Contains(t, body, `"npm run test *": "allow"`)
		assert.Contains(t, body, `"rm -rf *": "deny"`)
	})

	t.Run("one document for MCP servers and permissions", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cfg := permissionsConfig(dir)
		cfg.MCPServers = oneServer().MCPServers

		outputs, err := gen.Generate(&config.ContentTree{}, dir, cfg)

		require.NoError(t, err)
		count := 0
		for _, o := range outputs {
			if filepath.Base(o.Path) == "tool.jsonc" {
				count++
				assert.Contains(t, o.Content, `"mcp"`)
				assert.Contains(t, o.Content, `"permission"`)
				assert.True(t, o.PartiallyOwned || o.Content != "")
			}
		}
		assert.Equal(t, 1, count, "the sidecars of one document render once")
	})

	t.Run("no permissions, no document", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		outputs, err := gen.Generate(&config.ContentTree{}, dir, &config.Config{Name: "t", BaseDir: dir})

		require.NoError(t, err)
		assert.False(t, hasOutputPathSuffix(outputs, "tool.jsonc"), "emit_when defaults to has_permissions")
	})
}

func TestPermissionsSidecar_KeepsHandAuthoredRules(t *testing.T) {
	t.Parallel()

	gen := loadSpec(t, "name = \"tool\"\n[[sidecars]]\nkind = \"permissions\"\npath = \"tool.toml\"\ndialect = \"grok\"\n")
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tool.toml"), []byte("# mine\n[permission]\nallow = [\"Bash(make)\"] # keep\n"), 0o644))

	outputs, err := gen.Generate(&config.ContentTree{}, dir, permissionsConfig(dir))

	require.NoError(t, err)
	body, ok := findOutput(outputs, "tool.toml")
	require.True(t, ok)
	assert.Contains(t, body, "# mine")
	assert.Contains(t, body, "# keep")
	assert.Contains(t, body, "Bash(make)")
	assert.Contains(t, body, "Bash(npm run test)")
	assert.Contains(t, body, "Bash(rm -rf *)")
	for _, o := range outputs {
		if filepath.Base(o.Path) == "tool.toml" {
			assert.True(t, o.PartiallyOwned, "the file is shared with the user, so it must not be gitignored")
		}
	}
}

func TestBuiltinPermissionsSidecars(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"codebuddy": ".codebuddy/settings.json", "commandcode": ".commandcode/settings.json", "qoder": ".qoder/settings.json",
		"qwen": ".qwen/settings.json", "letta": ".letta/settings.json", "kilo": "kilo.jsonc",
		"mimocode": ".mimocode/mimocode.jsonc", "grok": ".grok/config.toml", "vibe": ".vibe/config.toml",
		"poolside": ".poolside/settings.yaml", "omp": ".omp/config.yml", "augment": ".augment/settings.json",
		"zoocode": ".vscode/settings.json", "copilot-cli": ".github/copilot/settings.json",
	}
	for name, path := range want {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			gen, err := providers.LoadBuiltin(name)
			require.NoError(t, err)
			dir := t.TempDir()
			cfg := permissionsConfig(dir)
			cfg.Presets = []config.Preset{{BuiltIn: name}}
			wantAllow, wantDeny := "npm run test", "rm -rf"
			if name == "copilot-cli" { // URL rules are all it can hold
				cfg.Permissions = &config.Permissions{Allow: []string{"WebFetch(domain:npm.dev)"}, Deny: []string{"WebFetch(domain:evil.dev)"}}
				wantAllow, wantDeny = "npm.dev", "evil.dev"
			}

			outputs, err := gen.Generate(&config.ContentTree{}, dir, cfg)

			require.NoError(t, err)
			body, ok := findOutput(outputs, path)
			require.True(t, ok, "%s renders %s", name, path)
			assert.Contains(t, body, wantAllow)
			assert.Contains(t, body, wantDeny)
		})
	}
}

func TestPermissionsSidecar_UserOnlyIsRenderedInUserScopeOnly(t *testing.T) {
	t.Parallel()

	gen := loadSpec(t, "name = \"tool\"\n[[sidecars]]\nkind = \"permissions\"\npath = \"p.json\"\ndialect = \"zed\"\n"+
		"global_path = \".config/tool/p.json\"\nuser_only = true\n")

	t.Run("project run", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		outputs, err := gen.Generate(&config.ContentTree{}, dir, permissionsConfig(dir))

		require.NoError(t, err)
		assert.False(t, hasOutputPathSuffix(outputs, "p.json"), "the tool ignores a project-level file, so none is written")
	})

	t.Run("user run", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cfg := permissionsConfig(dir)
		cfg.UserScope = true

		outputs, err := gen.Generate(&config.ContentTree{}, dir, cfg)

		require.NoError(t, err)
		body, ok := findOutput(outputs, "p.json")
		require.True(t, ok)
		assert.Contains(t, body, "always_deny")
	})
}
