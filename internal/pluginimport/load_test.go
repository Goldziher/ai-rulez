package pluginimport_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport"
	"github.com/Goldziher/ai-rulez/v5/internal/pluginimport/plugintest"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestMain(m *testing.M) {
	testutil.CeilGit()
	os.Exit(m.Run())
}

// loadDir writes files into a fresh directory and loads it as a plugin.
func loadDir(t *testing.T, files map[string]string, opt pluginimport.Options) (*pluginimport.Result, string) {
	t.Helper()
	dir := t.TempDir()
	testutil.WriteTree(t, dir, files)
	if opt.Skills == nil {
		opt.Skills = includes.ScanInstalledSkillDir
	}
	if opt.Wants == nil {
		opt.Wants = func(string) bool { return true }
	}
	res, err := pluginimport.Load(context.Background(), dir, &opt)
	require.NoError(t, err)
	return res, dir
}

func names(files []config.ContentFile) []string {
	out := make([]string, len(files))
	for i := range files {
		out[i] = files[i].Name
	}
	sort.Strings(out)
	return out
}

func serverNames(servers []config.MCPServer) []string {
	out := make([]string, len(servers))
	for i := range servers {
		out[i] = servers[i].Name
	}
	sort.Strings(out)
	return out
}

func TestLoad_Layouts(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		opt      pluginimport.Options
		format   string
		skills   []string
		agents   []string
		commands []string
		rules    []string
		hooks    int
		mcp      []string
	}{
		{
			name:  "claude default directories",
			files: plugintest.ClaudePlugin("acme"), opt: pluginimport.Options{EnableHooks: true, EnableMCP: true},
			format: config.PluginFormatClaude, skills: []string{"review-acme"}, agents: []string{"helper"}, commands: []string{"deploy"},
			hooks: 1, mcp: []string{"acme-docs"},
		},
		{
			name: "claude custom paths supplement the defaults",
			files: plugintest.Merge(plugintest.ClaudePlugin("acme"), map[string]string{
				".claude-plugin/plugin.json": `{"name": "acme", "skills": ["./extra-skills"], "agents": "./more-agents/", "commands": ["./cmds/one.md"],
					"hooks": "./config/hooks.json", "mcpServers": {"inline": {"command": "node"}}}`,
				"extra-skills/lint/SKILL.md": "---\nname: lint\ndescription: Lint\n---\n\nLint.\n",
				"more-agents/reviewer.md":    "---\nname: reviewer\ndescription: Reviews\n---\n\nReview.\n",
				"cmds/one.md":                "One.\n",
				"config/hooks.json":          `{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "echo hi"}]}]}}`,
			}),
			opt:    pluginimport.Options{EnableHooks: true, EnableMCP: true},
			format: config.PluginFormatClaude, skills: []string{"lint", "review-acme"}, agents: []string{"helper", "reviewer"},
			commands: []string{"deploy", "one"}, hooks: 2, mcp: []string{"acme-docs", "inline"},
		},
		{
			name: "claude plugin without a manifest",
			files: map[string]string{
				"commands/solo.md": "Solo.\n",
			},
			opt: pluginimport.Options{EnableMCP: true}, format: config.PluginFormatClaude, commands: []string{"solo"},
		},
		{
			name:  "codex",
			files: plugintest.CodexPlugin("tools"), opt: pluginimport.Options{EnableMCP: true},
			format: config.PluginFormatCodex, skills: []string{"codex-tools"}, mcp: []string{"tools-api"},
		},
		{
			name:  "gemini extension",
			files: plugintest.GeminiExtension("gem"), opt: pluginimport.Options{EnableMCP: true},
			format: config.PluginFormatGemini, skills: []string{"gem-gem"}, commands: []string{"git-commit", "review"},
			rules: []string{"gem-gemini"}, mcp: []string{"gem-srv"},
		},
		{
			name:  "cursor plugin",
			files: plugintest.CursorPlugin("cur"), opt: pluginimport.Options{EnableMCP: true},
			format: config.PluginFormatCursor, agents: []string{"planner"}, commands: []string{"ship"}, rules: []string{"style"},
		},
		{
			name:  "agent plugins package",
			files: plugintest.AgentPlugin("pkg"), opt: pluginimport.Options{EnableMCP: true},
			format: config.PluginFormatAgentPlugins, skills: []string{"ap-pkg"}, agents: []string{"ap-agent"}, mcp: []string{"pkg-ap"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			res, _ := loadDir(t, tt.files, tt.opt)

			// Assert
			assert.Equal(t, tt.format, res.Format)
			assert.Equal(t, orEmpty(tt.skills), names(res.Skills), "skills")
			assert.Equal(t, orEmpty(tt.agents), names(res.Agents), "agents")
			assert.Equal(t, orEmpty(tt.commands), names(res.Commands), "commands")
			assert.Equal(t, orEmpty(tt.rules), names(res.Rules), "rules")
			assert.Len(t, res.Hooks, tt.hooks, "hooks")
			assert.Equal(t, orEmpty(tt.mcp), serverNames(res.MCP), "mcp")
		})
	}
}

func orEmpty(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func TestLoad_SkillIsBundledWhole(t *testing.T) {
	// Arrange / Act
	res, dir := loadDir(t, plugintest.ClaudePlugin("acme"), pluginimport.Options{})

	// Assert
	require.Len(t, res.Skills, 1)
	skill := res.Skills[0]
	assert.Equal(t, filepath.Join(dir, "skills", "review-acme", "SKILL.md"), skill.Path)
	require.NotEmpty(t, skill.Resources)
	assert.Equal(t, "references/guide.md", skill.Resources[0].RelPath)
}
