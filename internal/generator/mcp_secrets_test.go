package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestLiteralSecrets(t *testing.T) {
	tests := []struct {
		name   string
		server config.MCPServer
		want   []string
	}{
		{"no credentials", config.MCPServer{URL: "https://example.com/mcp", Args: []string{"--port=80"}}, nil},
		{"password in user info", config.MCPServer{URL: "https://bot:hunter2hunter2@example.com/mcp"}, []string{"hunter2hunter2"}},
		{"token as user name", config.MCPServer{URL: "https://ghp_abcdefgh@example.com/mcp"}, []string{"ghp_abcdefgh"}},
		{"secret query value", config.MCPServer{URL: "https://example.com/mcp?token=abc123&page=2"}, []string{"abc123"}},
		{"encoded query value", config.MCPServer{URL: "https://example.com/mcp?api_key=a%2Fb"}, []string{"a%2Fb", "a/b"}},
		{"flag with equals", config.MCPServer{Args: []string{"--token=abc123", "--verbose"}}, []string{"abc123"}},
		{"flag with separate value", config.MCPServer{Args: []string{"--api-key", "abc123", "serve"}}, []string{"abc123"}},
		{"flag without a value", config.MCPServer{Args: []string{"--token", "--other"}}, nil},
		{"url in args", config.MCPServer{Args: []string{"https://u:pw12345678@host/x"}}, []string{"pw12345678"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, literalSecrets(&tt.server))
		})
	}
}

func TestMarkSensitiveOutputs_URLAndArgSecrets(t *testing.T) {
	cfg := &config.Config{BaseDir: t.TempDir(), MCPServers: map[string]*config.MCPServer{
		"remote": {URL: "https://bot:hunter2hunter2@example.com/mcp?token=q1w2e3r4t5"},
		"local":  {Command: "srv", Args: []string{"--api-key=0123456789abcdef"}},
	}}
	outputs := []config.OutputFile{
		{Path: filepath.Join(cfg.BaseDir, ".mcp.json"), Content: `{"url":"https://bot:hunter2hunter2@example.com/mcp"}`},
		{Path: filepath.Join(cfg.BaseDir, ".xum", "mcp.jsonc"), Content: `{"args":["--api-key=0123456789abcdef"]}`},
		{Path: filepath.Join(cfg.BaseDir, "NOTES.md"), Content: "no secrets"},
	}

	NewGenerator(cfg).markSensitiveOutputs(outputs)

	assert.True(t, outputs[0].Sensitive)
	assert.True(t, outputs[1].Sensitive)
	assert.False(t, outputs[2].Sensitive)
}

func TestEnsureSecretOutputsIgnored_URLCredentialsRequireAnIgnoredFile(t *testing.T) {
	dir := gitRepo(t, "")
	off := false
	gen := NewGenerator(&config.Config{
		BaseDir:    dir,
		Gitignore:  &off,
		MCPServers: map[string]*config.MCPServer{"s": {URL: "https://example.com/mcp?token=abc123"}},
	})
	outputs := []config.OutputFile{{Path: filepath.Join(dir, ".mcp.json")}, {Path: filepath.Join(dir, ".xum", "mcp.jsonc")}}

	err := gen.ensureSecretOutputsIgnored(outputs)

	require.Error(t, err)
	assert.Contains(t, err.Error(), ".mcp.json")
	assert.Contains(t, err.Error(), ".xum/mcp.jsonc", "every offending path is listed")
	assert.NotContains(t, err.Error(), "abc123")

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".mcp.json\n.xum/\n"), 0o644))
	assert.NoError(t, gen.ensureSecretOutputsIgnored(outputs))
}
