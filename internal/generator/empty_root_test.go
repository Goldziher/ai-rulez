package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pluginOnlyRootConfig is a nested root whose only declaration is a plugin: it
// keeps a preset but carries no rules, context, skills or agents, so its
// instruction file would be the generated header and nothing else.
func pluginOnlyRootConfig(agentsMD, claudeMD string) string {
	return "version = \"5.0\"\nname = \"plug\"\ngitignore = false\npresets = [\"claude\"]\n" + agentsMD + `
[plugin]
name = "plug"
version = "1.0.0"
description = "Plugin."
runtimes = ["claude"]

[plugin.author]
name = "tester"

[[plugin.mcp]]
name = "srv"
command = "echo"
` + claudeMD
}

// writeRootConfig writes only a config, so the root carries no content at all.
func writeRootConfig(t *testing.T, root, cfgTOML string) {
	t.Helper()
	path := filepath.Join(root, ".ai-rulez", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(cfgTOML), 0o644))
}

// TestGenerate_PluginOnlyRootSkipsHeaderOnlyInstructionFile pins #327: a root
// with no content writes no header-only CLAUDE.md/AGENTS.md and warns, naming
// the skipped file and the preset. The default shim and claude_md = "none" are
// both covered.
func TestGenerate_PluginOnlyRootSkipsHeaderOnlyInstructionFile(t *testing.T) {
	tests := []struct {
		name         string
		agentsMD     string
		claudeMD     string
		wantCLAUDE   bool
		wantAGENTS   bool
		wantWarnings []string
	}{
		{
			name:         "agents_md off writes no direct root file",
			agentsMD:     "agents_md = false\n",
			wantCLAUDE:   false,
			wantAGENTS:   false,
			wantWarnings: []string{"CLAUDE.md", "claude"},
		},
		{
			name:         "agents_md on writes no shim and no shared file",
			agentsMD:     "agents_md = true\n",
			wantCLAUDE:   false,
			wantAGENTS:   false,
			wantWarnings: []string{"CLAUDE.md", "claude"},
		},
		{
			// claude_md = "none" already omits the shim, so no root file is
			// rendered and no skip is reported; the empty AGENTS.md is skipped too.
			name:       "claude_md none writes no shim",
			agentsMD:   "agents_md = true\n",
			claudeMD:   "\n[claude]\nclaude_md = \"none\"\n",
			wantCLAUDE: false,
			wantAGENTS: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			warned := quietWarnings(t)
			writeRootConfig(t, root, pluginOnlyRootConfig(tc.agentsMD, tc.claudeMD))

			// Act
			runAgentsMDGenerate(t, root)

			// Assert
			assert.Equal(t, tc.wantCLAUDE, fileExists(filepath.Join(root, "CLAUDE.md")), "CLAUDE.md")
			assert.Equal(t, tc.wantAGENTS, fileExists(filepath.Join(root, "AGENTS.md")), "AGENTS.md")
			for _, want := range tc.wantWarnings {
				assert.Positive(t, countContaining(*warned, want), "a warning should name %q: %v", want, *warned)
			}
		})
	}
}

// TestGenerate_RootWithContentWritesInstructionFile pins the other half: a root
// with a rule keeps its instruction file, with and without the agents_md shim.
func TestGenerate_RootWithContentWritesInstructionFile(t *testing.T) {
	tests := []struct {
		name     string
		agentsMD string
	}{
		{name: "agents_md off", agentsMD: "agents_md = false\n"},
		{name: "agents_md shim", agentsMD: "agents_md = true\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			quietWarnings(t)
			writeRootConfig(t, root, "version = \"5.0\"\nname = \"p\"\ngitignore = false\npresets = [\"claude\"]\n"+
				tc.agentsMD+"\n[rules]\nmode = \"inline\"\n")
			writeAgentsMDFile(t, root, ".ai-rulez/rules/style.md", "# Style\n\nSTYLE_BODY\n")

			// Act
			runAgentsMDGenerate(t, root)

			// Assert
			require.FileExists(t, filepath.Join(root, "CLAUDE.md"))
			if tc.agentsMD == "agents_md = false\n" {
				assert.Contains(t, readAgentsMDFile(t, root, "CLAUDE.md"), "STYLE_BODY")
			} else {
				assert.Contains(t, readAgentsMDFile(t, root, "AGENTS.md"), "STYLE_BODY")
			}
		})
	}
}
