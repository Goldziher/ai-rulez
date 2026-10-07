package providers_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadProviderSpec_Hardening(t *testing.T) {
	t.Parallel()

	rulesBase := "name = \"t\"\n[root]\nfile = \"AGENTS.md\"\nsections = [\"rules_inline\"]\n"
	rules := rulesBase + "[outputs.rules]\nmode = \"per_item_file\"\ndir = \".x\"\nfilename = \"{id}.md\"\nsplit = true\n"
	tests := []struct {
		name    string
		toml    string
		wantErr string
	}{
		{"sidecar path traversal", "name = \"t\"\n[[sidecars]]\nkind = \"mcp\"\npath = \"../x.json\"", "sidecars[0].path"},
		{"sidecar absolute path", "name = \"t\"\n[[sidecars]]\nkind = \"mcp\"\npath = \"/etc/x.json\"", "sidecars[0].path"},
		{"legacy sidecar traversal", "name = \"t\"\n[[sidecars]]\nkind = \"mcp_json\"\npath = \"a/../../x.json\"", "sidecars[0].path"},
		{"root file traversal", "name = \"t\"\n[root]\nfile = \"../AGENTS.md\"", "root.file"},
		{"root file absolute", "name = \"t\"\n[root]\nfile = \"/AGENTS.md\"", "root.file"},
		{"directory traversal", "name = \"t\"\ndirectories = [\".ok\", \"../up\"]", "directories[1]"},
		{"output dir traversal", "name = \"t\"\n[outputs.skills]\nmode = \"per_item_file\"\ndir = \"../skills\"\nfilename = \"{id}.md\"", "outputs[\"skills\"].dir"},
		{"output dir absolute", "name = \"t\"\n[outputs.agents]\nmode = \"per_item_file\"\ndir = \"/abs\"\nfilename = \"{id}.md\"", "outputs[\"agents\"].dir"},
		{"root file in .git", "name = \"t\"\n[root]\nfile = \".git/config\"", "root.file"},
		{"root file in nested .GIT", "name = \"t\"\n[root]\nfile = \"sub/.GIT/hooks/pre-commit\"", "root.file"},
		{"root file in .ai-rulez", "name = \"t\"\n[root]\nfile = \".ai-rulez/config.toml\"", "root.file"},
		{"local file in .git", "name = \"t\"\n[root]\nfile = \"A.md\"\nlocal_file = \".git/config\"", "root.local_file"},
		{"sidecar in .git", "name = \"t\"\n[[sidecars]]\nkind = \"mcp\"\npath = \".git/hooks/post-checkout\"", "sidecars[0].path"},
		{"directory in .git", "name = \"t\"\ndirectories = [\".git/hooks\"]", "directories[0]"},
		{"output dir in .git", "name = \"t\"\n[outputs.skills]\nmode = \"per_item_file\"\ndir = \".git/hooks\"\nfilename = \"{id}.md\"", "outputs[\"skills\"].dir"},
		{"split dir in .git", rules[:len(rulesBase)] + "[outputs.rules]\nmode = \"per_item_file\"\ndir = \".git/hooks\"\nfilename = \"{id}.md\"\nsplit = true\ndialect = \"claude\"\n", ".git"},
		{"name uppercase", "name = \"Tool\"", "name"},
		{"name with slash", "name = \"a/b\"", "name"},
		{"name leading dash", "name = \"-a\"", "name"},
		{"permissions need a dialect", "name = \"t\"\n[[sidecars]]\nkind = \"permissions\"\npath = \"p.json\"", "needs a permissions dialect"},
		{"hooks need a dialect", "name = \"t\"\n[[sidecars]]\nkind = \"hooks\"\npath = \"h.json\"", "needs the name of a harness with hook support"},
		{"hooks with an unknown dialect", "name = \"t\"\n[[sidecars]]\nkind = \"hooks\"\npath = \"h.json\"\ndialect = \"nope\"", `got "nope"`},
		{"hooks with a key", "name = \"t\"\n[[sidecars]]\nkind = \"hooks\"\npath = \"h.json\"\ndialect = \"qwen\"\nkey = [\"x\"]", "takes no key or elements"},
		{"activation bad key", rules + "[outputs.rules.activation]\nalways = {\"a\\nb\" = \"x\"}", "must match"},
		{"activation key with colon", rules + "[outputs.rules.activation]\nalways = {\"a:b\" = \"x\"}", "must match"},
		{"embedded globs_list", rules + "[outputs.rules.activation]\nglob = {paths = \"x {globs_list}\"}", "whole value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			_, err := providers.LoadProviderSpec([]byte(tt.toml), "spec.toml", providers.FormatAuto)

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestBuiltinSpecNamesMatchFileStems(t *testing.T) {
	names, err := providers.BuiltinNames()
	require.NoError(t, err)
	for _, name := range names {
		gen, err := providers.LoadBuiltin(name)
		require.NoError(t, err, name)
		assert.Equal(t, name, gen.Spec.Name)
		assert.False(t, strings.ContainsAny(name, "/\\ "), name)
	}
}

func TestGlobalPaths_RelativeHomeAndEnv(t *testing.T) {
	t.Parallel()

	spec := loadSpec(t, "name = \"t\"\n[global]\nhome_env = \"T_HOME\"\nhome_dir = \".t\"\nroot_file = \".t/AGENTS.md\"\n").Spec

	// A relative home is not a user-scope root.
	assert.Nil(t, spec.GlobalPaths("relative/home", func(string) string { return "" }))

	// A relative env override is ignored with the home-based path used instead.
	g := spec.GlobalPaths(absSlash("/home/u"), func(string) string { return "rel/dir" })
	require.NotNil(t, g)
	assert.Equal(t, filepath.ToSlash(absSlash("/home/u/.t/AGENTS.md")), filepath.ToSlash(g.RootFile))

	g = spec.GlobalPaths(absSlash("/home/u"), func(string) string { return absSlash("/opt/t") })
	require.NotNil(t, g)
	assert.Equal(t, filepath.ToSlash(absSlash("/opt/t/AGENTS.md")), filepath.ToSlash(g.RootFile))
}

func TestSharedDirs(t *testing.T) {
	for _, d := range []string{".vscode", ".idea", ".zed", ".config", ".github", ".husky"} {
		assert.True(t, providers.IsSharedDir(d), d)
	}
	assert.False(t, providers.IsSharedDir(".claude"))
	assert.False(t, providers.IsSharedDir(".agents"), ".agents keeps its skills/ narrowing")
}

func TestMCPConfigPaths_ExplicitKindsOnly(t *testing.T) {
	// Arrange
	paths := providers.MCPConfigPaths()

	// Assert: derived from sidecar kinds, never from a file name containing "mcp".
	assert.Contains(t, paths, ".mcp.json")
	assert.True(t, providers.IsMCPSidecarKind("mcp"))
	for _, kind := range []string{"claude_settings_json", "mcp_json", "amp_settings_json", "pi_mcp_json"} {
		assert.True(t, providers.IsMCPSidecarKind(kind), kind)
	}
	for _, kind := range []string{"permissions", "hooks", "my_mcp_notes"} {
		assert.False(t, providers.IsMCPSidecarKind(kind), kind)
	}
}

func TestLoadProviderSpec_GlobalMCPPath(t *testing.T) {
	t.Parallel()

	_, err := providers.LoadProviderSpec([]byte("name = \"t\"\n[[sidecars]]\nkind = \"mcp\"\npath = \"a.json\"\nglobal_mcp_path = \"../x.json\""), "s.toml", providers.FormatAuto)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "global_mcp_path")

	_, err = providers.LoadProviderSpec([]byte("name = \"t\"\n[[sidecars]]\nkind = \"permissions\"\npath = \"a.json\"\nglobal_mcp_path = \".x.json\""), "s.toml", providers.FormatAuto)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only valid on a sidecar that holds MCP servers")
}
