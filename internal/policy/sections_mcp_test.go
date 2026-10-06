package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestParseMCPAndHooks(t *testing.T) {
	// Arrange
	body := "policy_version = 1\n[mcp]\nallowed_commands = [\"uvx\", \" npx \", \"npx\"]\ndeny_transports = [\"HTTP\", \"sse\", \"http\"]\n[hooks]\nallow = false\n"

	// Act
	_, p, err := Parse("p.toml", []byte(body))

	// Assert
	require.NoError(t, err)
	assert.Equal(t, List{Set: true, Items: []string{"npx", "uvx"}}, p.MCP.AllowedCommands)
	assert.Equal(t, []string{"http", "sse"}, p.MCP.DenyTransports)
	assert.True(t, p.Hooks.Forbidden)
}

func TestParseMCPAndHooksRejects(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"empty command", "policy_version = 1\n[mcp]\nallowed_commands = [\"\"]\n", "allowed_commands"},
		{"newline in command", "policy_version = 1\n[mcp]\nallowed_commands = [\"a\\nb\"]\n", "allowed_commands"},
		{"unknown transport", "policy_version = 1\n[mcp]\ndeny_transports = [\"ws\"]\n", "not a transport"},
		{"unknown mcp key", "policy_version = 1\n[mcp]\nallowed_command = []\n", "unknown key"},
		{"unknown hooks key", "policy_version = 1\n[hooks]\ndeny = true\n", "unknown key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, _, err := Parse("p.toml", []byte(tt.body))
			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), "AR743")
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestHooksAllowTrueConstrainsNothing(t *testing.T) {
	// Act
	_, p, err := Parse("p.toml", []byte("policy_version = 1\n[hooks]\nallow = true\n"))
	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"hooks.allow"}, p.statedLoose, "the explicit allow is remembered for the extends check")
	p.statedLoose = nil
	assert.Equal(t, Policy{}, p, "and constrains nothing")
}

func TestMergeMCPAndHooks(t *testing.T) {
	a := Policy{MCP: MCP{AllowedCommands: List{Set: true, Items: []string{"npx", "uvx"}}, DenyTransports: []string{"http"}}}
	b := Policy{MCP: MCP{AllowedCommands: List{Set: true, Items: []string{"uvx", "node"}}, DenyTransports: []string{"sse"}}, Hooks: Hooks{Forbidden: true}}

	got := Merge(a, b)

	assert.Equal(t, List{Set: true, Items: []string{"uvx"}}, got.MCP.AllowedCommands, "allowed commands intersect")
	assert.Equal(t, []string{"http", "sse"}, got.MCP.DenyTransports, "denied transports union")
	assert.True(t, got.Hooks.Forbidden, "forbidding hooks turns on")
	assert.Equal(t, got, Merge(b, a))
	assert.Equal(t, a.MCP, Merge(a, Policy{}).MCP, "unset constrains nothing")
}

func TestApplyMCPServers(t *testing.T) {
	servers := []config.MCPServer{
		{Name: "ok", Command: "npx"},
		{Name: "shell", Command: "bash"},
		{Name: "remote", Transport: "http", URL: "https://mcp.example.com"},
	}
	tests := []struct {
		name     string
		policy   MCP
		want     []string
		wantViol []string
	}{
		{"no constraint keeps all", MCP{}, []string{"ok", "shell", "remote"}, nil},
		{
			"allowed commands drop the other stdio servers; remote ones have no command",
			MCP{AllowedCommands: List{Set: true, Items: []string{"npx"}}},
			[]string{"ok", "remote"}, []string{"AR748 mcp.allowed_commands"},
		},
		{
			"an empty allowlist allows no stdio server",
			MCP{AllowedCommands: List{Set: true}},
			[]string{"remote"}, []string{"AR748 mcp.allowed_commands", "AR748 mcp.allowed_commands"},
		},
		{
			"a denied transport drops its servers",
			MCP{DenyTransports: []string{"http"}},
			[]string{"ok", "shell"}, []string{"AR748 mcp.deny_transports"},
		},
		{
			"stdio can be denied too",
			MCP{DenyTransports: []string{"stdio"}},
			[]string{"remote"}, []string{"AR748 mcp.deny_transports", "AR748 mcp.deny_transports"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := testConfig(t, "name = \"x\"\n")
			cfg.MCPServersRaw = append([]config.MCPServer(nil), servers...)
			res := Resolve([]Layer{layer("managed", Policy{MCP: tt.policy})})
			// Act
			out := res.Apply(cfg).Outcome
			// Assert
			var got []string
			for _, s := range cfg.MCPServersRaw {
				got = append(got, s.Name)
			}
			assert.Equal(t, tt.want, got)
			assert.ElementsMatch(t, tt.wantViol, codes(out))
		})
	}
}

func TestApplyHooksForbidden(t *testing.T) {
	// Arrange
	cfg := testConfig(t, "name = \"x\"\n")
	cfg.Hooks = []config.HookGroup{{Event: "PreToolUse"}, {Event: "SessionStart"}}
	res := Resolve([]Layer{layer("managed", Policy{Hooks: Hooks{Forbidden: true}})})
	// Act
	out := res.Apply(cfg).Outcome
	// Assert
	assert.Empty(t, cfg.Hooks)
	assert.ElementsMatch(t, []string{"AR748 hooks.allow", "AR748 hooks.allow"}, codes(out))
	assert.Equal(t, "managed", out.Violations[0].Origin)
}

func TestApplyHooksAllowedWithoutPolicyKeepsGroups(t *testing.T) {
	// Arrange
	cfg := testConfig(t, "name = \"x\"\n")
	cfg.Hooks = []config.HookGroup{{Event: "PreToolUse"}}
	res := Resolve([]Layer{layer("managed", Policy{Lock: Lock{Enforce: true}})})
	// Act
	out := res.Apply(cfg).Outcome
	// Assert
	assert.Len(t, cfg.Hooks, 1)
	assert.Empty(t, out.Violations)
}
