package policy

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Loaded metadata can be shared between loads (an include memo, a nested role or
// member load), so unloading a key must copy it, not edit it in place.
func TestApplyContent_DoesNotMutateTheSharedMetadata(t *testing.T) {
	// Arrange
	agent := contentFile(t, "vendor", filepath.Join(t.TempDir(), "agents", "vendor.md"), hookFrontmatter)
	shared := agent.Metadata
	cfg := contentConfig(t, agent)
	res := Resolve([]Layer{layer("managed", Policy{Hooks: Hooks{Forbidden: true}})})

	// Act
	got := res.ApplyContent(cfg)

	// Assert
	require.Len(t, got, 1)
	assert.Empty(t, cfg.Content.Agents[0].Metadata.ExecutingExtraKeys())
	assert.Equal(t, []string{"hooks"}, shared.ExecutingExtraKeys(), "a load without the policy still sees the hooks")
}

// A renderer passes these spellings through to a harness that reads them as
// executing or connecting (copilot writes mcp-servers, Claude include_extras
// writes every key), so the policy bounds each of them.
func TestApplyContent_BoundsEverySpellingOfAnExecutingKey(t *testing.T) {
	rogue := "  x:\n    command: curl-evil\n"
	tests := []struct {
		name        string
		frontmatter string
		policy      Policy
	}{
		{"hyphenated mcp-servers", "mcp-servers:\n" + rogue, Policy{MCP: MCP{AllowedCommands: List{Set: true, Items: []string{"npx"}}}}},
		{"snake case mcp_servers", "mcp_servers:\n" + rogue, Policy{MCP: MCP{AllowedCommands: List{Set: true, Items: []string{"npx"}}}}},
		{"upper case MCPServers", "MCPServers:\n" + rogue, Policy{MCP: MCP{AllowedCommands: List{Set: true, Items: []string{"npx"}}}}},
		{"capitalised Hooks", "Hooks:\n  PreToolUse: []\n", Policy{Hooks: Hooks{Forbidden: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			agent := contentFile(t, "vendor", filepath.Join(t.TempDir(), "agents", "vendor.md"), tt.frontmatter)
			require.NotEmpty(t, agent.Metadata.ExecutingExtraKeys(), "the fixture declares an executing key")
			cfg := contentConfig(t, agent)
			res := Resolve([]Layer{layer("managed", tt.policy)})

			// Act
			got := res.ApplyContent(cfg)

			// Assert
			require.Len(t, got, 1)
			assert.Equal(t, "AR748", got[0].Code)
			assert.Empty(t, cfg.Content.Agents[0].Metadata.ExecutingExtraKeys(), "the key is not loaded")
		})
	}
}
