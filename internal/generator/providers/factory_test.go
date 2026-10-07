package providers_test

import (
	"encoding/json"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func factoryGen(t *testing.T) *providers.Generator {
	t.Helper()
	gen, err := providers.LoadBuiltin("factory")
	require.NoError(t, err)
	return gen
}

func TestFactory_Generate(t *testing.T) {
	t.Parallel()

	content := &config.ContentTree{
		Rules:   []config.ContentFile{{Name: "be-nice", Content: "Always be nice.", Metadata: &config.Metadata{Priority: "high"}}},
		Context: []config.ContentFile{{Name: "layout", Content: "Use the module layout."}},
		Skills:  []config.ContentFile{{Name: "demo", Path: "/test/.ai-rulez/skills/demo/SKILL.md", Content: "Do demo things."}},
		Agents: []config.ContentFile{{
			Name: "reviewer", Content: "Review the diff.",
			Metadata: &config.Metadata{
				Tools: []string{"Read", "Grep"}, Effort: "xhigh",
				Extra: map[string]string{"description": "Reviews code"},
			},
		}},
		Commands: []config.ContentFile{
			{Name: "ship", Content: "Ship $ARGUMENTS.", Metadata: &config.Metadata{
				Extra: map[string]string{"description": "Ship it", "argument-hint": "<branch>"}}},
			{Name: "other-tool", Content: "Not for Factory.", Metadata: &config.Metadata{Targets: []string{"claude"}}},
		},
	}
	cfg := &config.Config{Name: "demo", Description: "Demo project.", BaseDir: "/test"}

	outputs, err := factoryGen(t).Generate(content, "/test", cfg)
	require.NoError(t, err)

	agentsMD := requireFile(t, outputs, "AGENTS.md")
	assert.Contains(t, agentsMD.Content, "Always be nice.")
	assert.Contains(t, agentsMD.Content, "Use the module layout.")

	skill := requireFile(t, outputs, ".factory/skills/demo/SKILL.md")
	assert.Equal(t, "demo", frontmatterValue(skill.Content, "name"))

	droid := requireFile(t, outputs, ".factory/droids/reviewer.md")
	assert.Equal(t, "reviewer", frontmatterValue(droid.Content, "name"))
	assert.Equal(t, "Reviews code", frontmatterValue(droid.Content, "description"))
	assert.Equal(t, "high", frontmatterValue(droid.Content, "reasoningEffort"), "tiers above high are capped")
	assert.Contains(t, droid.Content, "- Grep")
	assert.Contains(t, droid.Content, "Review the diff.")

	command := requireFile(t, outputs, ".factory/commands/ship.md")
	assert.Equal(t, "Ship it", frontmatterValue(command.Content, "description"))
	assert.Equal(t, "<branch>", frontmatterValue(command.Content, "argument-hint"))
	assert.Empty(t, frontmatterValue(command.Content, "name"), "the command name is the filename")
	assert.False(t, hasOutputPathSuffix(outputs, ".factory/commands/other-tool.md"), "targets exclude Factory")
}

// TestFactory_MCPJSON: type is only written for remote servers (it defaults to
// stdio).
func TestFactory_MCPJSON(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{Name: "demo", MCPServers: map[string]*config.MCPServer{
		"local": {Name: "local", Command: "npx", Args: []string{"-y", "pkg"}},
		"remote": {
			Name: "remote", Transport: config.TransportHTTP, URL: "https://example.com/mcp",
			Headers: map[string]string{"X-Key": "v"},
		},
	}}
	outputs, err := factoryGen(t).Generate(&config.ContentTree{}, "/test", cfg)
	require.NoError(t, err)

	var doc map[string]map[string]map[string]any
	require.NoError(t, json.Unmarshal([]byte(requireFile(t, outputs, ".factory/mcp.json").Content), &doc))
	require.Len(t, doc["mcpServers"], 2)
	assert.Equal(t, "npx", doc["mcpServers"]["local"]["command"])
	assert.NotContains(t, doc["mcpServers"]["local"], "type")
	assert.Equal(t, "http", doc["mcpServers"]["remote"]["type"])
	assert.Equal(t, "https://example.com/mcp", doc["mcpServers"]["remote"]["url"])

	outputs, err = factoryGen(t).Generate(&config.ContentTree{}, "/test", &config.Config{Name: "demo"})
	require.NoError(t, err)
	assert.False(t, hasOutputPathSuffix(outputs, ".factory/mcp.json"))
}

func TestFactory_GlobalPaths(t *testing.T) {
	t.Parallel()

	g := factoryGen(t).Spec.GlobalPaths(absSlash("/home/u"), func(string) string { return "" })
	require.NotNil(t, g)
	assert.Equal(t, absSlash("/home/u/.factory/AGENTS.md"), g.RootFile)
	assert.Equal(t, absSlash("/home/u/.factory/skills"), g.SkillsDir)
	assert.Equal(t, absSlash("/home/u/.factory/droids"), g.AgentsDir)
	assert.Equal(t, absSlash("/home/u/.factory/commands"), g.CommandsDir)
	assert.Equal(t, absSlash("/home/u/.factory/mcp.json"), g.Sidecars[".factory/mcp.json"])
}
