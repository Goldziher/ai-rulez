package lint

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rootComponentConfig is a single-plugin configuration: a [plugin] block with no
// monorepo members, so its bundle root is the repository root.
func rootComponentConfig(extra string) string {
	return baseConfig + "\n[plugin]\nname = \"p\"\nversion = \"1.0.0\"\n" + extra
}

// AR9T0 is a warning under the plugin analyzer's bundle scope.
func TestRootComponentRuleIsRegistered(t *testing.T) {
	rule, ok := lookupRule(CodeRootComponentShipped)
	require.True(t, ok, CodeRootComponentShipped)
	assert.Equal(t, "root-component-shipped", rule.Name)
	assert.Equal(t, SeverityWarning, rule.Default)
	assert.Equal(t, AnalyzerInfo{AnalyzerPlugin, ScopeBundle}, AnalyzerFor(CodeRootComponentShipped))
}

// AR9T0 warns for each default host component path that exists at the root of a
// single-plugin bundle whose root is the repository root, naming the host that
// loads it (#357).
func TestStrict_RootComponentShippedWarns(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name:  "mcp and settings files at the root",
			files: map[string]string{".mcp.json": "{}\n", "settings.json": "{}\n"},
			want:  []string{".mcp.json", "settings.json"},
		},
		{
			name:  "hooks, agents, commands and bin at the root",
			files: map[string]string{"hooks/hooks.json": "{}\n", "agents/a.md": "# a\n", "commands/c.md": "# c\n", "bin/tool": "#!/bin/sh\n"},
			want:  []string{"hooks", "agents", "commands", "bin"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := precisionTree("", "", nil)
			files[".ai-rulez/config.toml"] = rootComponentConfig("")
			for k, v := range tt.files {
				files[k] = v
			}
			root := t.TempDir()
			writeFiles(t, root, files)
			gitAdd(t, root)

			fs := lintDir(t, root)

			assert.Equal(t, len(tt.want), countCode(fs, CodeRootComponentShipped), "%s", dump(fs))
			for _, suffix := range tt.want {
				assert.True(t, has(fs, CodeRootComponentShipped, suffix, 0), "%s must be flagged:\n%s", suffix, dump(fs))
			}
			for _, f := range fs {
				if f.Code == CodeRootComponentShipped {
					assert.Contains(t, f.Message, "loads it", "the message names the host that loads the path: %q", f.Message)
					assert.Contains(t, f.Message, "content_root", "the message offers the fix: %q", f.Message)
				}
			}
		})
	}
}

// AR9T0 does not fire when the bundle root is not the repository root, or when
// ai-rulez itself writes the path for the targeted runtime (#357).
func TestStrict_RootComponentShippedStaysQuiet(t *testing.T) {
	tests := []struct {
		name   string
		config string
		files  map[string]string
	}{
		{
			name:   "no plugin block",
			config: baseConfig,
			files:  map[string]string{".mcp.json": "{}\n"},
		},
		{
			name:   "monorepo root",
			config: baseConfig + "\n[plugin]\nname = \"p\"\nversion = \"1.0.0\"\n\n[marketplace]\nname = \"m\"\nmembers = [\"plugins/a\"]\n",
			files:  map[string]string{".mcp.json": "{}\n"},
		},
		{
			name:   "no root-layout runtime",
			config: baseConfig + "\n[plugin]\nname = \"p\"\nversion = \"1.0.0\"\nruntimes = [\"opencode\"]\n",
			files:  map[string]string{".mcp.json": "{}\n"},
		},
		{
			name:   "agents the bundle ships",
			config: rootComponentConfig(""),
			files:  map[string]string{".ai-rulez/agents/x.md": "---\nname: x\ndescription: A bundled agent. Use when testing.\n---\nbody\n", "agents/x.md": "# x\n"},
		},
		{
			name:   "codex root writes its own mcp.json",
			config: rootComponentConfig("\nruntimes = [\"codex\"]\n\n[plugin.codex]\nmanifest = \"root\"\n\n[[plugin.mcp]]\nname = \"s\"\ncommand = \"npx\"\n"),
			files:  map[string]string{"mcp.json": "{}\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := precisionTree("", "", nil)
			files[".ai-rulez/config.toml"] = tt.config
			for k, v := range tt.files {
				files[k] = v
			}
			root := t.TempDir()
			writeFiles(t, root, files)
			gitAdd(t, root)

			fs := lintDir(t, root)

			assert.Zero(t, countCode(fs, CodeRootComponentShipped), "%s", dump(fs))
		})
	}
}

// A Claude-only plugin inlines its MCP servers into plugin.json, so a root
// .mcp.json is the project's own file and still ships to every user.
func TestStrict_RootComponentShippedClaudeMCPFileStillWarns(t *testing.T) {
	root := t.TempDir()
	files := precisionTree("", "", nil)
	files[".ai-rulez/config.toml"] = rootComponentConfig("\nruntimes = [\"claude\"]\n\n[[plugin.mcp]]\nname = \"s\"\ncommand = \"npx\"\n")
	files[".mcp.json"] = "{}\n"
	writeFiles(t, root, files)
	gitAdd(t, root)

	fs := lintDir(t, root)

	assert.True(t, has(fs, CodeRootComponentShipped, ".mcp.json", 0), "%s", dump(fs))
}

// A content_root moves the plugin's commands and agents out of the scanned tree,
// so the check does not guess whether the root commands/ and agents/ directories
// are ai-rulez's own; it still warns about the paths ai-rulez never writes.
func TestStrict_RootComponentShippedWithContentRoot(t *testing.T) {
	root := t.TempDir()
	files := precisionTree("", "", nil)
	files[".ai-rulez/config.toml"] = rootComponentConfig("content_root = \"plugin\"\n")
	files[".mcp.json"] = "{}\n"
	files["commands/c.md"] = "# c\n"
	writeFiles(t, root, files)
	gitAdd(t, root)

	fs := lintDir(t, root)

	assert.True(t, has(fs, CodeRootComponentShipped, ".mcp.json", 0), "%s", dump(fs))
	assert.False(t, has(fs, CodeRootComponentShipped, "commands", 0), "content_root makes the payload ambiguous:\n%s", dump(fs))
}

// AR9T0 looks for component paths only under the plugin's own root: a .mcp.json
// in a parent directory (the repository toplevel above a nested plugin) is not
// shipped by it (#378), while one inside the plugin directory still is.
func TestStrict_RootComponentIgnoresParentDirectories(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  int
	}{
		{"mcp only in the parent directory", map[string]string{".mcp.json": "{}\n"}, 0},
		{"mcp inside the plugin directory", map[string]string{"pkg/.mcp.json": "{}\n"}, 1},
		{"mcp at both levels", map[string]string{".mcp.json": "{}\n", "pkg/.mcp.json": "{}\n"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			files := map[string]string{}
			for k, v := range precisionTree("", "", nil) {
				files["pkg/"+k] = v
			}
			files["pkg/.ai-rulez/config.toml"] = rootComponentConfig("")
			for k, v := range tt.files {
				files[k] = v
			}
			root := t.TempDir()
			writeFiles(t, root, files)
			gitAdd(t, root)

			// Act
			fs := lintDir(t, filepath.Join(root, "pkg"))

			// Assert
			assert.Equal(t, tt.want, countCode(fs, CodeRootComponentShipped), "%s", dump(fs))
		})
	}
}
