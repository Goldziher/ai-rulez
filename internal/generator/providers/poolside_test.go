package providers_test

import (
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func poolsideGen(t *testing.T) *providers.Generator {
	t.Helper()
	gen, err := providers.LoadBuiltin("poolside")
	require.NoError(t, err)
	return gen
}

func TestPoolside_Generate(t *testing.T) {
	t.Parallel()

	content := &config.ContentTree{
		Rules:  []config.ContentFile{{Name: "be-nice", Content: "Always be nice.", Metadata: &config.Metadata{Priority: "high"}}},
		Skills: []config.ContentFile{{Name: "demo", Path: "/test/.ai-rulez/skills/demo/SKILL.md", Content: "Do demo things."}},
	}
	outputs, err := poolsideGen(t).Generate(content, "/test", &config.Config{Name: "demo", BaseDir: "/test"})
	require.NoError(t, err)

	assert.Contains(t, requireFile(t, outputs, "AGENTS.md").Content, "Always be nice.")
	skill := requireFile(t, outputs, ".poolside/skills/demo/SKILL.md")
	assert.Equal(t, "demo", frontmatterValue(skill.Content, "name"))
}

// TestPoolside_SettingsYAML: servers go under mcp_servers of the committed
// settings file; remote ones carry a transport block with "Name: value" headers,
// and a hand-written key survives.
func TestPoolside_SettingsYAML(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	require.NoError(t, mkdirAndWrite(filepath.Join(baseDir, ".poolside", "settings.yaml"), "theme: dark\nmcp_servers:\n  mine:\n    command: mine\n    args: []\n"))
	cfg := &config.Config{Name: "demo", BaseDir: baseDir, MCPServers: map[string]*config.MCPServer{
		"local": {Name: "local", Command: "npx", Args: []string{"-y", "pkg"}, Env: map[string]string{"A": "B"}},
		"remote": {
			Name: "remote", Transport: config.TransportHTTP, URL: "https://example.com/mcp",
			Headers: map[string]string{"Authorization": "Bearer t"},
		},
	}}
	outputs, err := poolsideGen(t).Generate(&config.ContentTree{}, baseDir, cfg)
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(requireFile(t, outputs, ".poolside/settings.yaml").Content), &doc))
	assert.Equal(t, "dark", doc["theme"], "user keys are preserved")
	servers, ok := doc["mcp_servers"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, servers, "mine")
	assert.Equal(t, map[string]any{"command": "npx", "args": []any{"-y", "pkg"}, "env": map[string]any{"A": "B"}}, servers["local"])
	assert.Equal(t, map[string]any{
		"transport": map[string]any{"type": "http", "url": "https://example.com/mcp", "headers": []any{"Authorization: Bearer t"}},
	}, servers["remote"])
}

func TestPoolside_NoServersNoSettings(t *testing.T) {
	t.Parallel()

	outputs, err := poolsideGen(t).Generate(&config.ContentTree{}, "/test", &config.Config{Name: "demo"})
	require.NoError(t, err)
	assert.False(t, hasOutputPathSuffix(outputs, ".poolside/settings.yaml"))
}

func TestPoolside_GlobalPaths(t *testing.T) {
	t.Parallel()

	g := poolsideGen(t).Spec.GlobalPaths("/home/u", func(string) string { return "" })
	require.NotNil(t, g)
	assert.Equal(t, filepath.FromSlash("/home/u/.config/poolside/AGENTS.md"), g.RootFile)
	assert.Equal(t, filepath.FromSlash("/home/u/.config/poolside/skills"), g.SkillsDir)
	assert.Equal(t, filepath.FromSlash("/home/u/.config/poolside/settings.yaml"), g.Sidecars[".poolside/settings.yaml"])
}
