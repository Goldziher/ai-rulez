package providers_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func codebuffGen(t *testing.T) *providers.Generator {
	t.Helper()
	gen, err := providers.LoadBuiltin("codebuff")
	require.NoError(t, err)
	return gen
}

// TestCodebuff_Outputs: AGENTS.md (its knowledge file), skills and the MCP document.
// Codebuff's strict
// schema takes stdio {type, command, args, env} and remote {type, url, headers}.
func TestCodebuff_Outputs(t *testing.T) {
	t.Parallel()

	content := &config.ContentTree{
		Rules:  []config.ContentFile{{Name: "be-nice", Content: "Always be nice."}},
		Skills: []config.ContentFile{{Name: "demo", Path: "/test/.ai-rulez/skills/demo/SKILL.md", Content: "x"}},
	}
	disabled := false
	cfg := &config.Config{Name: "demo", BaseDir: absSlash("/test"), MCPServers: map[string]*config.MCPServer{
		"local": {Name: "local", Command: "npx", Args: []string{"-y", "pkg"}, Env: map[string]string{"A": "B"}, Description: "ignored"},
		"remote": {
			Name: "remote", Transport: config.TransportHTTP, URL: "https://example.com/mcp",
			Headers: map[string]string{"Authorization": "Bearer t"},
		},
		"off": {Name: "off", Command: "x", Enabled: &disabled},
	}}

	outputs, err := codebuffGen(t).Generate(content, absSlash("/test"), cfg)
	require.NoError(t, err)

	var files []string
	for _, o := range outputs {
		if !o.IsDir {
			files = append(files, filepath.ToSlash(o.Path))
		}
	}
	base := filepath.ToSlash(absSlash("/test"))
	assert.ElementsMatch(t, []string{base + "/AGENTS.md", base + "/.agents/skills/demo/SKILL.md", base + "/.agents/mcp.json"}, files)
	agentsMD := requireFile(t, outputs, "AGENTS.md")
	assert.Contains(t, agentsMD.Content, "Always be nice.", "rules are inlined into the knowledge file")

	var doc map[string]map[string]map[string]any
	require.NoError(t, json.Unmarshal([]byte(requireFile(t, outputs, ".agents/mcp.json").Content), &doc))
	servers := doc["mcpServers"]
	require.Len(t, servers, 2, "a disabled server has no Codebuff equivalent and is left out")
	assert.Equal(t, map[string]any{
		"type": "stdio", "command": "npx", "args": []any{"-y", "pkg"}, "env": map[string]any{"A": "B"},
	}, servers["local"])
	assert.Equal(t, map[string]any{
		"type": "http", "url": "https://example.com/mcp", "headers": map[string]any{"Authorization": "Bearer t"},
	}, servers["remote"])
}

func TestCodebuff_NoServersNoOutput(t *testing.T) {
	t.Parallel()

	outputs, err := codebuffGen(t).Generate(&config.ContentTree{}, "/test", &config.Config{Name: "demo"})
	require.NoError(t, err)
	assert.False(t, hasOutputPathSuffix(outputs, ".agents/mcp.json"))
}

func TestCodebuff_GlobalPaths(t *testing.T) {
	t.Parallel()

	g := codebuffGen(t).Spec.GlobalPaths(absSlash("/home/u"), func(string) string { return "" })
	require.NotNil(t, g)
	assert.Equal(t, absSlash("/home/u/.agents/mcp.json"), g.Sidecars[".agents/mcp.json"])
}
