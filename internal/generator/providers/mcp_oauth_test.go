package providers

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
)

func oauthServers() *config.Config {
	return &config.Config{MCPServers: map[string]*config.MCPServer{
		"slack": {
			Name: "slack", Type: config.TransportHTTP, URL: "https://mcp.slack.com/mcp",
			OAuth: &config.MCPOAuth{ClientID: "cid", CallbackPort: 8080, Scopes: []string{"channels:read"}},
		},
		"plain": {Name: "plain", Command: "npx", Args: []string{"-y", "pkg"}},
	}}
}

// TestMCPDialectEntries_OAuthRendersForSupportingDialect: a dialect whose tool
// reads an oauth block renders clientId, callbackPort and scopes, while a plain
// server is untouched.
func TestMCPDialectEntries_OAuthRendersForSupportingDialect(t *testing.T) {
	t.Parallel()

	// Arrange
	dialect, err := mcpDialectFor(MCPDialectVSCode)
	require.NoError(t, err)

	// Act
	entries := mcpDialectEntries(dialect, oauthServers())

	// Assert
	got, err := json.Marshal(entries)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"slack": {"type": "http", "url": "https://mcp.slack.com/mcp", "oauth": {"clientId": "cid", "callbackPort": 8080, "scopes": ["channels:read"]}},
		"plain": {"type": "stdio", "command": "npx", "args": ["-y", "pkg"]}}`, string(got))
}

// TestMCPDialectEntries_OAuthSkippedWithWarning: a dialect whose tool has no
// oauth block drops the server and says so, rather than writing one without the
// client it needs.
func TestMCPDialectEntries_OAuthSkippedWithWarning(t *testing.T) {
	t.Parallel()

	// Arrange
	dialect, err := mcpDialectFor(MCPDialectGemini)
	require.NoError(t, err)
	var warnings []string
	restore := diag.SetDefaultSink(func(msg string, args ...any) { warnings = append(warnings, msg+" "+fmt.Sprint(args...)) })
	t.Cleanup(restore)

	// Act
	entries := mcpDialectEntries(dialect, oauthServers())

	// Assert
	assert.NotContains(t, entries, "slack", "an oauth server is skipped by a dialect that cannot express it")
	assert.Contains(t, entries, "plain")
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "oauth")
	assert.Contains(t, warnings[0], "slack")
}

// TestMCPDialectEntries_CodexOAuth pins the Codex config.toml oauth table.
func TestMCPDialectEntries_CodexOAuth(t *testing.T) {
	t.Parallel()

	// Arrange
	dialect, err := mcpDialectFor(MCPDialectCodex)
	require.NoError(t, err)
	cfg := &config.Config{MCPServers: map[string]*config.MCPServer{
		"slack": {
			Name: "slack", Type: config.TransportHTTP, URL: "https://mcp.slack.com/mcp",
			OAuth: &config.MCPOAuth{ClientID: "cid", CallbackPort: 8080, Scopes: []string{"channels:read"}},
		},
	}}

	// Act
	got, err := toml.Marshal(map[string]any{"mcp_servers": mcpDialectEntries(dialect, cfg)})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, `[mcp_servers]
[mcp_servers.slack]
scopes = ['channels:read']
url = 'https://mcp.slack.com/mcp'

[mcp_servers.slack.oauth]
callback_port = 8080
client_id = 'cid'
`, string(got))
}

// TestMCPJSONServerEntries_OAuth: the shared root .mcp.json (read by Claude Code
// and VS Code) renders the camelCase oauth block for a hosted server.
func TestMCPJSONServerEntries_OAuth(t *testing.T) {
	t.Parallel()

	// Act
	entries := mcpJSONServerEntries(oauthServers(), true)

	// Assert
	got, err := json.Marshal(entries)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"slack": {"type": "http", "url": "https://mcp.slack.com/mcp", "oauth": {"clientId": "cid", "callbackPort": 8080, "scopes": ["channels:read"]}},
		"plain": {"type": "stdio", "command": "npx", "args": ["-y", "pkg"]}}`, string(got))
}

// TestMCPJSONServerEntries_NoOAuthUnchanged: a server without an oauth block
// renders exactly as it did before the field existed.
func TestMCPJSONServerEntries_NoOAuthUnchanged(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := &config.Config{MCPServers: map[string]*config.MCPServer{
		"plain": {Name: "plain", Command: "npx", Args: []string{"-y", "pkg"}},
	}}

	// Act
	entries := mcpJSONServerEntries(cfg, true)

	// Assert
	got, err := json.Marshal(entries)
	require.NoError(t, err)
	assert.JSONEq(t, `{"plain": {"type": "stdio", "command": "npx", "args": ["-y", "pkg"]}}`, string(got))
}
