package generator

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const scopedFallbackSkill = "\n[rules]\nscoped_fallback = \"skill\"\n"

func TestScopedFallback_SkillMovesGlobItemsOutOfAgentsMD(t *testing.T) {
	root := newAgentsMDFoldersProject(t, "agents_md = true\n", []string{"claude", "codex"}, scopedFallbackSkill)
	runAgentsMDGenerate(t, root)

	agents := readAgentsMDFile(t, root, "AGENTS.md")
	assert.NotContains(t, agents, "GO_BODY", "the glob rule leaves AGENTS.md")
	assert.NotContains(t, agents, "SCOPEDCTX_BODY", "the glob context leaves AGENTS.md")
	assert.Contains(t, agents, "AUTO_BODY", "codex has no folder for auto rules, so they stay inline")
	assert.Contains(t, agents, "MANUAL_BODY")
	assert.Equal(t, "@AGENTS.md", afterBanner(readAgentsMDFile(t, root, "CLAUDE.md")))

	rule := readAgentsMDFile(t, root, ".agents/skills/rule-go-style/SKILL.md")
	assert.Contains(t, rule, "name: rule-go-style\n")
	assert.Contains(t, rule, "description: \"go-style: conventions for files matching **/*.go.")
	assert.Contains(t, rule, "paths:\n  - '**/*.go'\n")
	assert.Contains(t, rule, "GO_BODY")
	assert.Contains(t, readAgentsMDFile(t, root, ".agents/skills/context-scoped/SKILL.md"), "SCOPEDCTX_BODY")

	assert.Contains(t, readAgentsMDFile(t, root, ".claude/rules/go-style.md"), "GO_BODY", "claude keeps its path-scoped rule")
	assert.NoFileExists(t, filepath.Join(root, ".claude", "skills", "rule-go-style", "SKILL.md"))
}

func TestScopedFallback_DefaultInlinesAsBefore(t *testing.T) {
	root := newAgentsMDFoldersProject(t, "agents_md = true\n", []string{"claude", "codex"}, "")
	runAgentsMDGenerate(t, root)

	assert.Contains(t, readAgentsMDFile(t, root, "AGENTS.md"), "GO_BODY")
	assert.NoDirExists(t, filepath.Join(root, ".agents", "skills", "rule-go-style"))
}

func TestScopedFallback_StaysInlineForAReaderWithoutTheSkillsTree(t *testing.T) {
	// zcode reads AGENTS.md but not .agents/skills, so a skill would never reach it.
	root := newAgentsMDFoldersProject(t, "agents_md = true\n", []string{"claude", "codex", "zcode"}, scopedFallbackSkill)
	runAgentsMDGenerate(t, root)

	assert.Contains(t, readAgentsMDFile(t, root, "AGENTS.md"), "GO_BODY")
	assert.NoDirExists(t, filepath.Join(root, ".agents", "skills", "rule-go-style"))
}

func TestScopedFallback_AuthoredSkillKeepsItsName(t *testing.T) {
	root := newAgentsMDFoldersProject(t, "agents_md = true\n", []string{"codex"}, scopedFallbackSkill)
	writeAgentsMDFile(t, root, ".ai-rulez/skills/rule-go-style/SKILL.md",
		"---\nname: rule-go-style\ndescription: authored\n---\nAUTHORED_BODY\n")
	runAgentsMDGenerate(t, root)

	skill := readAgentsMDFile(t, root, ".agents/skills/rule-go-style/SKILL.md")
	assert.Contains(t, skill, "AUTHORED_BODY")
	assert.NotContains(t, skill, "GO_BODY")
}

func TestScopedFallback_RejectsUnknownValue(t *testing.T) {
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"codex"}, "", "\n[rules]\nscoped_fallback = \"nested\"\n"))
	cfg, err := config.LoadConfig(context.Background(), root)
	if err == nil {
		err = cfg.Validate()
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scoped_fallback")
}
