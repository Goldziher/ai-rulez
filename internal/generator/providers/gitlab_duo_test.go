package providers_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitlabDuoGen(t *testing.T) *providers.Generator {
	t.Helper()
	gen, err := providers.LoadBuiltin("gitlab-duo")
	require.NoError(t, err)
	return gen
}

func TestGitlabDuo_Generate(t *testing.T) {
	t.Parallel()

	content := &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "be-nice", Content: "Always be nice.", Metadata: &config.Metadata{Priority: "high"}},
			// Duo has no rules folder, so a scoped rule stays in the one rules file.
			{Name: "go-only", Content: "GO_RULE", Metadata: &config.Metadata{Globs: []string{"**/*.go"}}},
		},
		Context: []config.ContentFile{{Name: "layout", Content: "Use the module layout."}},
		Skills:  []config.ContentFile{{Name: "demo", Path: "/test/.ai-rulez/skills/demo/SKILL.md", Content: "x"}},
		Commands: []config.ContentFile{{Name: "report", Content: "Prepare a daily report.",
			Metadata: &config.Metadata{Extra: map[string]string{"description": "Daily report"}}}},
	}
	cfg := &config.Config{Name: "demo", BaseDir: "/test", Rules: &config.RulesConfig{Mode: config.RulesModeSplit}}

	outputs, err := gitlabDuoGen(t).Generate(content, "/test", cfg)
	require.NoError(t, err)

	rules := requireFile(t, outputs, ".gitlab/duo/chat-rules.md")
	assert.Contains(t, rules.Content, "Always be nice.")
	assert.Contains(t, rules.Content, "GO_RULE")
	assert.Contains(t, rules.Content, "Use the module layout.")

	command := requireFile(t, outputs, ".agents/commands/report.md")
	assert.Equal(t, "Daily report", frontmatterValue(command.Content, "description"))
	assert.Empty(t, frontmatterValue(command.Content, "name"))
	assert.Contains(t, command.Content, "Prepare a daily report.")

	assert.False(t, hasOutputPathContains(outputs, "skills/"), "skills are only documented inside Duo plugins")
}

// TestGitlabDuo_MCPJSON: every documented entry spells the type out.
func TestGitlabDuo_MCPJSON(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{Name: "demo", MCPServers: map[string]*config.MCPServer{
		"local":  {Name: "local", Command: "npx", Args: []string{"-y", "pkg"}},
		"remote": {Name: "remote", Transport: config.TransportSSE, URL: "https://example.com/sse"},
	}}
	outputs, err := gitlabDuoGen(t).Generate(&config.ContentTree{}, "/test", cfg)
	require.NoError(t, err)

	var doc map[string]map[string]map[string]any
	require.NoError(t, json.Unmarshal([]byte(requireFile(t, outputs, ".gitlab/duo/mcp.json").Content), &doc))
	assert.Equal(t, "stdio", doc["mcpServers"]["local"]["type"])
	assert.Equal(t, "sse", doc["mcpServers"]["remote"]["type"])
	assert.Equal(t, "https://example.com/sse", doc["mcpServers"]["remote"]["url"])

	outputs, err = gitlabDuoGen(t).Generate(&config.ContentTree{}, "/test", &config.Config{Name: "demo"})
	require.NoError(t, err)
	assert.False(t, hasOutputPathSuffix(outputs, ".gitlab/duo/mcp.json"))
}

func TestGitlabDuo_GlobalPaths(t *testing.T) {
	t.Parallel()

	g := gitlabDuoGen(t).Spec.GlobalPaths("/home/u", func(string) string { return "" })
	require.NotNil(t, g)
	assert.Equal(t, filepath.FromSlash("/home/u/.gitlab/duo/chat-rules.md"), g.RootFile)
	assert.Equal(t, filepath.FromSlash("/home/u/.gitlab/duo/commands"), g.CommandsDir)
	assert.Equal(t, filepath.FromSlash("/home/u/.gitlab/duo/mcp.json"), g.Sidecars[".gitlab/duo/mcp.json"])
}
