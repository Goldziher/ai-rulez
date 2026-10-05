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

func backfillServers() map[string]*config.MCPServer {
	return map[string]*config.MCPServer{
		"local":  {Name: "local", Command: "npx", Args: []string{"-y", "x"}, Env: map[string]string{"A": "b"}},
		"remote": {Name: "remote", Transport: config.TransportHTTP, URL: "https://example.com/mcp"},
	}
}

func backfillCommands() *config.ContentTree {
	return &config.ContentTree{Commands: []config.ContentFile{
		{Name: "ship", Content: "Ship it.", Metadata: &config.Metadata{Extra: map[string]string{"description": "Ship a release"}}},
	}}
}

func generated(t *testing.T, preset string, content *config.ContentTree, cfg *config.Config) map[string]string {
	t.Helper()
	gen, err := providers.LoadBuiltin(preset)
	require.NoError(t, err)
	base := t.TempDir()
	outputs, err := gen.Generate(content, base, cfg)
	require.NoError(t, err)
	files := map[string]string{}
	for _, o := range outputs {
		if o.IsDir {
			continue
		}
		rel, relErr := filepath.Rel(base, o.Path)
		require.NoError(t, relErr)
		files[filepath.ToSlash(rel)] = o.Content
	}
	return files
}

func TestAmp_SettingsCarryMCPServersAndEffort(t *testing.T) {
	tests := []struct {
		name       string
		cfg        *config.Config
		wantFile   bool
		wantEffort any
		wantKeys   []string
	}{
		{"mcp servers only", &config.Config{Name: "p", MCPServers: backfillServers()}, true, nil, []string{"local", "remote"}},
		{
			"effort and mcp servers",
			&config.Config{Name: "p", MCPServers: backfillServers(), Defaults: &config.DefaultsConfig{Effort: "high"}},
			true, "high", []string{"local", "remote"},
		},
		{"effort only", &config.Config{Name: "p", Defaults: &config.DefaultsConfig{Effort: "high"}}, true, "high", nil},
		{"neither", &config.Config{Name: "p"}, false, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := generated(t, "amp", &config.ContentTree{}, tt.cfg)

			body, ok := files[".amp/settings.json"]
			require.Equal(t, tt.wantFile, ok)
			if !ok {
				return
			}
			var doc map[string]any
			require.NoError(t, json.Unmarshal([]byte(body), &doc))
			assert.Equal(t, tt.wantEffort, doc["amp.anthropic.effort"])
			servers, _ := doc["amp.mcpServers"].(map[string]any)
			if tt.wantKeys == nil {
				assert.Nil(t, doc["amp.mcpServers"])
				return
			}
			for _, name := range tt.wantKeys {
				assert.Contains(t, servers, name)
			}
			local := servers["local"].(map[string]any)
			assert.Equal(t, "npx", local["command"])
			assert.Equal(t, map[string]any{"A": "b"}, local["env"])
			assert.Equal(t, "https://example.com/mcp", servers["remote"].(map[string]any)["url"])
		})
	}
}

func TestJunie_MCPAndCommands(t *testing.T) {
	files := generated(t, "junie", backfillCommands(), &config.Config{Name: "p", MCPServers: backfillServers()})

	var doc map[string]map[string]map[string]any
	require.NoError(t, json.Unmarshal([]byte(files[".junie/mcp/mcp.json"]), &doc))
	assert.Equal(t, "npx", doc["mcpServers"]["local"]["command"])
	assert.Equal(t, "https://example.com/mcp", doc["mcpServers"]["remote"]["url"])

	cmd := files[".junie/commands/ship.md"]
	assert.Contains(t, cmd, "description: Ship a release")
	assert.Contains(t, cmd, "Ship it.")
}

func TestJunie_NoMCPServersNoDocument(t *testing.T) {
	files := generated(t, "junie", &config.ContentTree{}, &config.Config{Name: "p"})
	assert.NotContains(t, files, ".junie/mcp/mcp.json")
}

func TestPi_PromptsFromCommands(t *testing.T) {
	files := generated(t, "pi", backfillCommands(), &config.Config{Name: "p"})

	prompt := files[".pi/prompts/ship.md"]
	assert.Contains(t, prompt, "description: Ship a release")
	assert.Contains(t, prompt, "Ship it.")
	assert.NotContains(t, prompt, "name:")
}
