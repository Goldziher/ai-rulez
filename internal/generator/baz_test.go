package generator

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	bazScopedRule = "---\npaths:\n  - services/api/**/*.py\n---\n# API Style\n\nAPI_STYLE_BODY\n"
	bazRootRule   = "---\npaths:\n  - \"**/*.sql\"\n---\n# Sql Style\n\nSQL_STYLE_BODY\n"
)

func bazProjectFiles(t *testing.T, root, cfgTOML string) {
	t.Helper()
	writeAgentsMDProject(t, root, cfgTOML)
	writeAgentsMDFile(t, root, ".ai-rulez/rules/api-style.md", bazScopedRule)
	writeAgentsMDFile(t, root, ".ai-rulez/rules/sql-style.md", bazRootRule)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "services", "api"), 0o755))
}

func readBazFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	require.NoError(t, err, rel)
	return string(data)
}

func TestBaz_ComposesWithClaudeAndCodex(t *testing.T) {
	tests := []struct {
		name    string
		presets []string
		flag    string
	}{
		{"baz alone", []string{"baz"}, ""},
		{"baz and claude", []string{"claude", "baz"}, ""},
		{"baz and claude with agents_md", []string{"claude", "baz"}, "agents_md = true\n"},
		{"baz, claude and codex", []string{"claude", "codex", "baz"}, ""},
		{"baz, claude and codex with agents_md", []string{"claude", "codex", "baz"}, "agents_md = true\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			bazProjectFiles(t, root, agentsMDConfig(tt.presets, tt.flag, ""))
			runAgentsMDGenerate(t, root)

			rootMD := readBazFile(t, root, "AGENTS.md")
			assert.Contains(t, rootMD, "ALWAYS_BODY")
			assert.Contains(t, rootMD, "SQL_STYLE_BODY", "a glob without a directory stays in the root file")
			assert.NotContains(t, rootMD, "API_STYLE_BODY", "the scoped rule moved to its directory")

			nested := readBazFile(t, root, "services/api/AGENTS.md")
			assert.Contains(t, nested, "API_STYLE_BODY")
			assert.NotContains(t, nested, "ALWAYS_BODY")

			// Commands and .claude/rules are not something Baz reads; baz adds neither.
			_, err := os.Stat(filepath.Join(root, ".agents", "commands"))
			assert.True(t, os.IsNotExist(err))

			claude, codex := false, false
			for _, p := range tt.presets {
				claude = claude || p == "claude"
				codex = codex || p == "codex"
			}
			_, skillsErr := os.Stat(filepath.Join(root, ".agents", "skills", "alpha", "SKILL.md"))
			_, claudeSkillsErr := os.Stat(filepath.Join(root, ".claude", "skills", "alpha", "SKILL.md"))
			if claude && tt.flag == "" {
				assert.NoError(t, claudeSkillsErr)
				if codex {
					assert.NoError(t, skillsErr, "codex writes the skill to .agents/skills")
				} else {
					assert.True(t, os.IsNotExist(skillsErr), "claude already provides the skill at .claude/skills")
				}
			}
		})
	}
}

func TestBaz_ClaudeOutputUnchangedByBaz(t *testing.T) {
	without := t.TempDir()
	bazProjectFiles(t, without, agentsMDConfig([]string{"claude"}, "", ""))
	runAgentsMDGenerate(t, without)

	with := t.TempDir()
	bazProjectFiles(t, with, agentsMDConfig([]string{"claude", "baz"}, "", ""))
	runAgentsMDGenerate(t, with)

	for _, rel := range []string{".claude/skills/alpha/SKILL.md", ".claude/agents/helper.md", ".claude/rules/always.md"} {
		assert.Equal(t, stripHeaderHashes(readBazFile(t, without, rel)), stripHeaderHashes(readBazFile(t, with, rel)), rel)
	}
}

func TestBaz_RootModeKeepsScopedRuleInRootFile(t *testing.T) {
	root := t.TempDir()
	bazProjectFiles(t, root, agentsMDConfig([]string{"baz"}, "", "\n[rules]\nbaz_scoped = \"root\"\n"))
	runAgentsMDGenerate(t, root)

	assert.Contains(t, readBazFile(t, root, "AGENTS.md"), "API_STYLE_BODY")
	_, err := os.Stat(filepath.Join(root, "services", "api", "AGENTS.md"))
	assert.True(t, os.IsNotExist(err))
}

func TestBaz_StaleNestedFileIsRemovedWhenRuleGoes(t *testing.T) {
	root := t.TempDir()
	bazProjectFiles(t, root, agentsMDConfig([]string{"baz"}, "", ""))
	runAgentsMDGenerate(t, root)
	require.FileExists(t, filepath.Join(root, "services", "api", "AGENTS.md"))

	require.NoError(t, os.Remove(filepath.Join(root, ".ai-rulez", "rules", "api-style.md")))
	runAgentsMDGenerate(t, root)
	assert.NoFileExists(t, filepath.Join(root, "services", "api", "AGENTS.md"))
}

func TestBaz_NeverWritesIntoTheConfigDirectory(t *testing.T) {
	root := t.TempDir()
	bazProjectFiles(t, root, agentsMDConfig([]string{"baz"}, "", ""))
	writeAgentsMDFile(t, root, ".ai-rulez/rules/self.md", "---\npaths:\n  - .ai-rulez/rules/**\n---\n# Self\n\nSELF_BODY\n")
	runAgentsMDGenerate(t, root)
	runAgentsMDGenerate(t, root)

	assert.NoFileExists(t, filepath.Join(root, ".ai-rulez", "rules", "AGENTS.md"))
	assert.Contains(t, readBazFile(t, root, "AGENTS.md"), "SELF_BODY")
}

var headerHashLine = regexp.MustCompile(`(?m)^#? ?(Content|Source)-Hash:.*$`)

func stripHeaderHashes(s string) string {
	return headerHashLine.ReplaceAllString(s, "")
}
