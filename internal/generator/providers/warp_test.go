package providers_test

import (
	"encoding/json"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func warpGen(t *testing.T) *providers.Generator {
	t.Helper()
	gen, err := providers.LoadBuiltin("warp")
	require.NoError(t, err)
	return gen
}

// TestWarp_Generate: rules inline in AGENTS.md, skills and commands (as skills)
// under .warp/skills.
func TestWarp_Generate(t *testing.T) {
	t.Parallel()

	content := &config.ContentTree{
		Rules: []config.ContentFile{{Name: "be-nice", Content: "Always be nice.", Metadata: &config.Metadata{Priority: "high"}}},
		Skills: []config.ContentFile{{Name: "demo", Path: "/test/.ai-rulez/skills/demo/SKILL.md", Content: "Do demo things.",
			Metadata: &config.Metadata{Extra: map[string]string{"description": "Demo skill"}}}},
		Commands: []config.ContentFile{
			{Name: "review", Content: "Review $ARGUMENTS.", Metadata: &config.Metadata{Extra: map[string]string{"description": "Review a PR"}}},
			{Name: "claude-only", Content: "x", Metadata: &config.Metadata{Targets: []string{"claude"}}},
		},
	}
	cfg := &config.Config{Name: "demo", BaseDir: "/test"}

	outputs, err := warpGen(t).Generate(content, "/test", cfg)
	require.NoError(t, err)

	assert.Contains(t, requireFile(t, outputs, "AGENTS.md").Content, "Always be nice.")

	skill := requireFile(t, outputs, ".warp/skills/demo/SKILL.md")
	assert.Equal(t, "Demo skill", frontmatterValue(skill.Content, "description"))

	command := requireFile(t, outputs, ".warp/skills/review/SKILL.md")
	assert.Equal(t, "review", frontmatterValue(command.Content, "name"))
	assert.Equal(t, "Review a PR", frontmatterValue(command.Content, "description"))
	assert.Contains(t, command.Content, "Review $ARGUMENTS.")
	assert.False(t, hasOutputPathSuffix(outputs, ".warp/skills/claude-only/SKILL.md"))
}

func TestWarp_MCPJSON(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{Name: "demo", MCPServers: map[string]*config.MCPServer{
		"fs": {Name: "fs", Command: "npx", Args: []string{"-y", "pkg"}, Env: map[string]string{"A": "B"}},
	}}
	outputs, err := warpGen(t).Generate(&config.ContentTree{}, "/test", cfg)
	require.NoError(t, err)

	var doc map[string]map[string]map[string]any
	require.NoError(t, json.Unmarshal([]byte(requireFile(t, outputs, ".warp/.mcp.json").Content), &doc))
	assert.Equal(t, "npx", doc["mcpServers"]["fs"]["command"])
	assert.Equal(t, map[string]any{"A": "B"}, doc["mcpServers"]["fs"]["env"])

	outputs, err = warpGen(t).Generate(&config.ContentTree{}, "/test", &config.Config{Name: "demo"})
	require.NoError(t, err)
	assert.False(t, hasOutputPathSuffix(outputs, ".warp/.mcp.json"))
}

func TestWarp_GlobalPaths(t *testing.T) {
	t.Parallel()

	g := warpGen(t).Spec.GlobalPaths(absSlash("/home/u"), func(string) string { return "" })
	require.NotNil(t, g)
	assert.Equal(t, absSlash("/home/u/.agents/AGENTS.md"), g.RootFile)
	assert.Equal(t, absSlash("/home/u/.warp/skills"), g.SkillsDir)
	assert.Equal(t, absSlash("/home/u/.warp/.mcp.json"), g.Sidecars[".warp/.mcp.json"])
}
