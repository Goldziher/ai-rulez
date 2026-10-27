package generator

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const scopedFallbackDirectory = "\n[rules]\nscoped_fallback = \"skill\"\nscoped_fallback_group = \"directory\"\n"

func TestScopedFallback_DirectoryGroupFoldsItemsIntoOneSkill(t *testing.T) {
	root := newAgentsMDFoldersProject(t, "agents_md = true\n", []string{"claude", "codex"}, scopedFallbackDirectory)
	writeAgentsMDFile(t, root, ".ai-rulez/rules/web-ts.md", "---\nglobs: [\"web/**/*.ts\"]\n---\nWEBTS_BODY\n")
	writeAgentsMDFile(t, root, ".ai-rulez/rules/web-css.md", "---\nglobs: [\"web/**/*.css\"]\n---\nWEBCSS_BODY\n")
	runAgentsMDGenerate(t, root)

	group := readAgentsMDFile(t, root, ".agents/skills/scoped-web/SKILL.md")
	assert.Contains(t, group, "name: scoped-web\n")
	for _, glob := range []string{"web/**/*.ts", "web/**/*.css", "web/**"} {
		assert.Contains(t, group, "  - "+glob+"\n")
	}
	for _, body := range []string{"WEBTS_BODY", "WEBCSS_BODY", "SCOPEDCTX_BODY"} {
		assert.Contains(t, group, body)
	}
	assert.Contains(t, group, "## web-ts\n\n_Applies to: `web/**/*.ts`_\n")
	assert.Less(t, len(skillDescription(t, group)), 200, "a grouped description stays short")

	assert.NoDirExists(t, filepath.Join(root, ".agents", "skills", "rule-web-ts"))
	assert.NoDirExists(t, filepath.Join(root, ".agents", "skills", "context-scoped"))
	assert.FileExists(t, filepath.Join(root, ".agents", "skills", "rule-go-style", "SKILL.md"), "a lone item keeps its own skill")
	assert.NotContains(t, readAgentsMDFile(t, root, "AGENTS.md"), "WEBTS_BODY")
}

func TestScopedFallback_OptOutKeepsAnItemForPathAwareHarnessesOnly(t *testing.T) {
	root := newAgentsMDFoldersProject(t, "agents_md = true\n", []string{"claude", "codex"}, scopedFallbackSkill)
	writeAgentsMDFile(t, root, ".ai-rulez/rules/claude-only.md", "---\nglobs: [\"web/**\"]\nfallback: false\n---\nCLAUDEONLY_BODY\n")
	runAgentsMDGenerate(t, root)

	assert.NotContains(t, readAgentsMDFile(t, root, "AGENTS.md"), "CLAUDEONLY_BODY")
	assert.NoDirExists(t, filepath.Join(root, ".agents", "skills", "rule-claude-only"))
	claude := readAgentsMDFile(t, root, ".claude/rules/claude-only.md")
	assert.Contains(t, claude, "CLAUDEONLY_BODY")
	assert.NotContains(t, claude, "fallback")
}

func TestScopedFallback_OptOutAlsoLeavesTheInlineAgentsMD(t *testing.T) {
	root := newAgentsMDFoldersProject(t, "agents_md = true\n", []string{"claude", "codex"}, "")
	writeAgentsMDFile(t, root, ".ai-rulez/rules/claude-only.md", "---\nglobs: [\"web/**\"]\nfallback: false\n---\nCLAUDEONLY_BODY\n")
	runAgentsMDGenerate(t, root)

	agents := readAgentsMDFile(t, root, "AGENTS.md")
	assert.NotContains(t, agents, "CLAUDEONLY_BODY")
	assert.Contains(t, agents, "GO_BODY", "items without the opt-out stay inline")
}

func TestScopedFallback_RejectsUnknownGroup(t *testing.T) {
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"codex"}, "", "\n[rules]\nscoped_fallback_group = \"all\"\n"))
	cfg, err := config.LoadConfig(context.Background(), root)
	if err == nil {
		err = cfg.Validate()
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scoped_fallback_group")
}

func TestScopedFallback_IncludeGroupFoldsAnIncludesItems(t *testing.T) {
	extra := "\n[rules]\nscoped_fallback = \"skill\"\nscoped_fallback_group = \"include\"\n" +
		"\n[[includes]]\nname = \"shared\"\nsource = \"./shared-inc\"\ninclude = [\"rules\"]\n"
	root := newAgentsMDFoldersProject(t, "agents_md = true\n", []string{"claude", "codex"}, extra)
	writeAgentsMDFile(t, root, "shared-inc/.ai-rulez/rules/inc-a.md", "---\nglobs: [\"a/**\"]\n---\nINCA_BODY\n")
	writeAgentsMDFile(t, root, "shared-inc/.ai-rulez/rules/inc-b.md", "---\nglobs: [\"b/**\"]\n---\nINCB_BODY\n")
	cfg, err := loadWithResolvers(context.Background(), root)
	require.NoError(t, err)
	require.NoError(t, NewGenerator(cfg).Generate(""))

	group := readAgentsMDFile(t, root, ".agents/skills/scoped-shared/SKILL.md")
	assert.Contains(t, group, "INCA_BODY")
	assert.Contains(t, group, "INCB_BODY")
	assert.Contains(t, group, "  - a/**\n")
	assert.Contains(t, group, "  - b/**\n")
	assert.NotContains(t, group, "GO_BODY", "the project's own items are not part of an include's skill")
	assert.FileExists(t, filepath.Join(root, ".agents", "skills", "rule-go-style", "SKILL.md"))
}

// skillDescription returns the description value of a rendered SKILL.md.
func skillDescription(t *testing.T, skill string) string {
	t.Helper()
	for _, line := range strings.Split(skill, "\n") {
		if rest, ok := strings.CutPrefix(line, "description: "); ok {
			return rest
		}
	}
	t.Fatalf("no description in %q", skill)
	return ""
}
