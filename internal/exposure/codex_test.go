package exposure

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/harnesslimits"
)

func buildCodex(t *testing.T, files map[string]string) *Inventory {
	t.Helper()
	root := writeRepo(t, files)
	return build(t, root, func(o *Options) { o.Harness = HarnessCodex })
}

func TestBuild_CodexReadsItsOwnArtifactsOnly(t *testing.T) {
	// Arrange
	inv := buildCodex(t, map[string]string{
		"AGENTS.md":                     "# root agents\n",
		"CLAUDE.md":                     "@AGENTS.md\n",
		"svc/AGENTS.md":                 "shadowed by the override\n",
		"svc/AGENTS.override.md":        "override\n",
		".claude/rules/r.md":            "claude rule",
		".claude/settings.json":         `{"enabledPlugins": {"p@m": true}}`,
		".claude/skills/c/SKILL.md":     skillMD("c", "A claude skill."),
		".mcp.json":                     `{"mcpServers": {"claude-only": {}}}`,
		".agents/skills/x/SKILL.md":     skillMD("x", "Codex skill x.", "paths:\n  - nothing/**\n"),
		"svc/.agents/skills/y/SKILL.md": skillMD("y", "Codex skill y."),
		".codex/config.toml":            "[mcp_servers.a]\ncommand = \"a\"\n[mcp_servers.b]\nurl = \"https://b\"\n[mcp_servers.off]\ncommand = \"o\"\nenabled = false\n",
	})

	// Assert
	assert.Equal(t, HarnessCodex, inv.Harness)
	assert.Equal(t, []string{"AGENTS.md", "svc/AGENTS.override.md"}, paths(inv.Artifacts, KindAgentsMD))
	assert.Equal(t, PhaseStart, named(t, inv, KindAgentsMD, "AGENTS.md").Phase)
	assert.Equal(t, PhaseOnDemand, named(t, inv, KindAgentsMD, "svc/AGENTS.override.md").Phase)
	assert.Empty(t, paths(inv.Artifacts, KindClaudeMD, KindRule, KindSettings, KindPlugin, KindAgent))
	assert.Equal(t, []string{".agents/skills/x/SKILL.md", "svc/.agents/skills/y/SKILL.md"}, paths(inv.Artifacts, KindSkill))
	x := named(t, inv, KindSkill, "x")
	assert.Equal(t, PhaseStart, x.Phase, "a claude paths: gate does not apply to codex")
	assert.Equal(t, VisibilityAlways, x.Visibility)
	assert.Equal(t, "x: Codex skill x. (.agents/skills/x/SKILL.md)", x.ListingText)
	assert.Equal(t, PhaseOnDemand, named(t, inv, KindSkill, "y").Phase)
	assert.Equal(t, []string{"a", "b"}, names(inv, KindMCPServer))
	assert.Equal(t, ".codex/config.toml", named(t, inv, KindMCPServer, "a").Path)
	assert.Equal(t, PhaseStart, named(t, inv, KindMCPServer, "a").Phase)
}

func TestBuild_CodexOverrideShadowsAgentsMDInTheSameDirectory(t *testing.T) {
	inv := buildCodex(t, map[string]string{"AGENTS.md": "base\n", "AGENTS.override.md": "override text\n"})

	assert.Equal(t, []string{"AGENTS.override.md"}, paths(inv.Artifacts, KindAgentsMD))
}

func TestBuild_CodexAgentsMDIsCappedAtTheDocumentedLimit(t *testing.T) {
	limit := harnesslimits.MustValue("codex.agents_md_chain_bytes")
	inv := buildCodex(t, map[string]string{"AGENTS.md": strings.Repeat("a", limit+5000)})

	a := named(t, inv, KindAgentsMD, "AGENTS.md")

	assert.Equal(t, EstimateTokens(limit), a.Tokens)
	assert.True(t, a.Truncated)
}

func TestBuild_CodexSkillDescriptionIsNotCappedAtClaudesLimit(t *testing.T) {
	long := strings.Repeat("word ", 500)
	inv := buildCodex(t, map[string]string{".agents/skills/long/SKILL.md": skillMD("long", long)})

	a := named(t, inv, KindSkill, "long")

	assert.False(t, a.Truncated)
	assert.Greater(t, a.ListingChars, listingCap())
}

func TestBuild_ClaudeIgnoresCodexArtifacts(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"AGENTS.md": "x", ".agents/skills/x/SKILL.md": skillMD("x", "d"), ".codex/config.toml": "[mcp_servers.a]\n",
	})

	inv := build(t, root)

	assert.Equal(t, HarnessClaude, inv.Harness)
	assert.Empty(t, inv.Artifacts)
}

func TestBuild_RejectsAnUnknownHarness(t *testing.T) {
	_, err := Build(context.Background(), Options{Root: t.TempDir(), Files: []string{}, Harness: "gemini"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown harness "gemini"`)
}

func TestListingBudgetCharsFor(t *testing.T) {
	assert.Equal(t, 8000, ListingBudgetCharsFor(HarnessClaude, 200000))
	assert.Equal(t, 16000, ListingBudgetCharsFor(HarnessCodex, 200000))
}
