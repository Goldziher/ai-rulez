package providers_test

import (
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lettaGen(t *testing.T) *providers.Generator {
	t.Helper()
	gen, err := providers.LoadBuiltin("letta")
	require.NoError(t, err)
	return gen
}

// TestLetta_Generate: subagents under .letta/agents with comma-separated lists
// (Letta's frontmatter parser ignores YAML lists), skills in .agents/skills, and
// no root file, rules or MCP document.
func TestLetta_Generate(t *testing.T) {
	t.Parallel()

	content := &config.ContentTree{
		Rules:  []config.ContentFile{{Name: "be-nice", Content: "Always be nice."}},
		Skills: []config.ContentFile{{Name: "demo", Path: "/test/.ai-rulez/skills/demo/SKILL.md", Content: "Do demo things."}},
		Agents: []config.ContentFile{{
			Name: "explorer", Content: "Explore the code.",
			Metadata: &config.Metadata{
				Tools: []string{"Read", "Grep"}, Skills: []string{"demo", "other"},
				Extra: map[string]string{"description": "Explores code"},
			},
		}},
	}
	cfg := &config.Config{Name: "demo", BaseDir: "/test", MCPServers: map[string]*config.MCPServer{"fs": {Name: "fs", Command: "npx"}}}

	outputs, err := lettaGen(t).Generate(content, "/test", cfg)
	require.NoError(t, err)

	agent := requireFile(t, outputs, ".letta/agents/explorer.md")
	assert.Equal(t, "explorer", frontmatterValue(agent.Content, "name"))
	assert.Equal(t, "Explores code", frontmatterValue(agent.Content, "description"))
	assert.Equal(t, "Read, Grep", frontmatterValue(agent.Content, "tools"))
	assert.Equal(t, "demo, other", frontmatterValue(agent.Content, "skills"))
	assert.Contains(t, agent.Content, "Explore the code.")

	skill := requireFile(t, outputs, ".agents/skills/demo/SKILL.md")
	assert.Equal(t, "demo", frontmatterValue(skill.Content, "name"))

	for _, o := range outputs {
		assert.False(t, !o.IsDir && filepath.Base(o.Path) == "AGENTS.md", "letta writes no root file")
	}
	assert.False(t, hasOutputPathContains(outputs, "mcp"), "letta has no MCP file")
}

func TestLetta_GlobalPaths(t *testing.T) {
	t.Parallel()

	g := lettaGen(t).Spec.GlobalPaths(absSlash("/home/u"), func(string) string { return "" })
	require.NotNil(t, g)
	assert.Equal(t, absSlash("/home/u/.letta/agents"), g.AgentsDir)
	assert.Equal(t, absSlash("/home/u/.letta/skills"), g.SkillsDir)
}

// TestLetta_JoinListsOffByDefault: the other presets keep writing YAML lists.
func TestLetta_JoinListsOffByDefault(t *testing.T) {
	t.Parallel()

	gen, err := providers.LoadBuiltin("junie")
	require.NoError(t, err)
	content := &config.ContentTree{Agents: []config.ContentFile{{
		Name: "scout", Content: "x", Metadata: &config.Metadata{Tools: []string{"read", "grep"}},
	}}}
	outputs, err := gen.Generate(content, "/test", &config.Config{Name: "demo", BaseDir: "/test"})
	require.NoError(t, err)

	assert.Contains(t, requireFile(t, outputs, ".junie/agents/scout.md").Content, "- grep")
}

// TestLetta_Commands: custom slash commands are .commands/{id}.md, named by file.
func TestLetta_Commands(t *testing.T) {
	t.Parallel()

	content := &config.ContentTree{Commands: []config.ContentFile{{
		Name: "ship", Content: "Ship $ARGUMENTS.",
		Metadata: &config.Metadata{Extra: map[string]string{"description": "Ship it", "argument-hint": "<env>"}},
	}}}

	outputs, err := lettaGen(t).Generate(content, "/test", &config.Config{Name: "demo", BaseDir: "/test"})
	require.NoError(t, err)

	cmd := requireFile(t, outputs, ".commands/ship.md")
	assert.Equal(t, "Ship it", frontmatterValue(cmd.Content, "description"))
	assert.Equal(t, "<env>", frontmatterValue(cmd.Content, "argument-hint"))
	assert.Empty(t, frontmatterValue(cmd.Content, "name"), "a command takes its name from the file")
	assert.Contains(t, cmd.Content, "Ship $ARGUMENTS.")
}
