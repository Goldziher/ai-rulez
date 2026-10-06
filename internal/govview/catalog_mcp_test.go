package govview

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestCatalogMCPServers(t *testing.T) {
	// Arrange
	off := false
	cfg := &config.Config{MCPServersRaw: []config.MCPServer{
		{Name: "zeta", Command: "/usr/local/bin/zeta-mcp", Args: []string{"--token", "hunter2"}},
		{Name: "github", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"},
			Env: map[string]string{"GITHUB_TOKEN": "${GITHUB_TOKEN}", "API_KEY": "sk-live-abc123", "REGION": "eu"}},
		{Name: "pinned", Command: "uvx", Args: []string{"tool==1.2.3"}, Enabled: &off, Profiles: []string{"b", "a"}},
		{Name: "remote", Transport: config.TransportHTTP, URL: "https://user:pw@internal.example/mcp",
			Headers: map[string]string{"Authorization": "Bearer abc", "X-Ref": "${REMOTE_KEY}"}},
		{Name: "digest", Command: "docker", Args: []string{"run", "img@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}},
		{Name: "scoped", Command: "npx", Args: []string{"@scope/pkg@2.0.1"}},
		{Name: "latest", Command: "npx", Args: []string{"pkg@latest"}},
	}}

	// Act
	got := catalogMCPServers(cfg)

	// Assert
	byName := map[string]CatalogMCPServer{}
	var names []string
	for _, s := range got {
		byName[s.Name] = s
		names = append(names, s.Name)
	}
	assert.Equal(t, []string{"digest", "github", "latest", "pinned", "remote", "scoped", "zeta"}, names)

	gh := byName["github"]
	assert.Equal(t, "mcp/github", gh.Ref)
	assert.Equal(t, "npx", gh.CommandBasename)
	require.NotNil(t, gh.Pinned)
	assert.False(t, *gh.Pinned)
	assert.Equal(t, []MCPValue{
		{Name: "API_KEY", Literal: true},
		{Name: "GITHUB_TOKEN", Ref: "GITHUB_TOKEN"},
		{Name: "REGION", Literal: true},
	}, gh.Env)
	assert.Len(t, gh.Warnings, 2, "unpinned launch and the literal credential")

	assert.Equal(t, "zeta-mcp", byName["zeta"].CommandBasename)
	assert.Nil(t, byName["zeta"].Pinned, "a local executable has no pin")
	assert.Nil(t, byName["remote"].Pinned)
	assert.Equal(t, "http", byName["remote"].Transport)
	assert.Equal(t, []MCPValue{{Name: "Authorization", Literal: true}, {Name: "X-Ref", Ref: "REMOTE_KEY"}}, byName["remote"].Headers)
	assert.False(t, byName["pinned"].Enabled)
	assert.Equal(t, []string{"a", "b"}, byName["pinned"].Profiles)
	for _, name := range []string{"pinned", "scoped", "digest"} {
		require.NotNil(t, byName[name].Pinned, name)
		assert.True(t, *byName[name].Pinned, name)
	}
	require.NotNil(t, byName["latest"].Pinned)
	assert.False(t, *byName["latest"].Pinned)
}

func TestCatalogMCPServersNeverCarrySecrets(t *testing.T) {
	// Arrange
	cfg := &config.Config{MCPServersRaw: []config.MCPServer{
		{Name: "a", Command: "/home/me/secret-dir/run", Args: []string{"--password=pw-arg"},
			Env: map[string]string{"TOKEN": "tok-value"}, Description: "ok"},
		{Name: "b", Transport: config.TransportSSE, URL: "https://user:pw-url@internal.example/x",
			Headers: map[string]string{"Authorization": "hdr-value"}},
	}}

	// Act
	data, err := json.Marshal(catalogMCPServers(cfg))

	// Assert
	require.NoError(t, err)
	for _, leak := range []string{"pw-arg", "tok-value", "pw-url", "internal.example", "hdr-value", "secret-dir", "/home/me"} {
		assert.NotContainsf(t, string(data), leak, "%q must not reach the catalog", leak)
	}
}

func TestCommandBasename(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""}, {"npx", "npx"}, {"/usr/bin/node", "node"}, {`C:\tools\mcp.exe`, "mcp.exe"}, {"node server.js --key k", "node"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, commandBasename(tt.in))
		})
	}
}
