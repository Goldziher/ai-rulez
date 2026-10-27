package config

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }

func TestSelfMCPServerEntry(t *testing.T) {
	tests := []struct {
		name string
		mcp  *MCPConfig
		want map[string]any
	}{
		{
			name: "defaults to latest so a project picks up new releases",
			mcp:  &MCPConfig{SelfServer: boolPtr(true)},
			want: map[string]any{"type": "stdio", "command": "npx", "args": []string{"-y", "ai-rulez@latest", "mcp"}},
		},
		{
			name: "no [mcp] table also defaults to latest",
			mcp:  nil,
			want: map[string]any{"type": "stdio", "command": "npx", "args": []string{"-y", "ai-rulez@latest", "mcp"}},
		},
		{
			name: "a dev version resolves to latest",
			mcp:  &MCPConfig{SelfServer: boolPtr(true), SelfServerVersion: "dev"},
			want: map[string]any{"type": "stdio", "command": "npx", "args": []string{"-y", "ai-rulez@latest", "mcp"}},
		},
		{
			name: "a configured version pins it",
			mcp:  &MCPConfig{SelfServer: boolPtr(true), SelfServerVersion: "4.1.0"},
			want: map[string]any{"type": "stdio", "command": "npx", "args": []string{"-y", "ai-rulez@4.1.0", "mcp"}},
		},
		{
			name: "command override replaces npx entirely",
			mcp:  &MCPConfig{SelfServer: boolPtr(true), SelfServerCommand: []string{"ai-rulez", "mcp"}},
			want: map[string]any{"type": "stdio", "command": "ai-rulez", "args": []string{"mcp"}},
		},
		{
			name: "command override without arguments omits args",
			mcp:  &MCPConfig{SelfServer: boolPtr(true), SelfServerCommand: []string{"ai-rulez-mcp"}},
			want: map[string]any{"type": "stdio", "command": "ai-rulez-mcp"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{MCP: tt.mcp}
			assert.Equal(t, tt.want, cfg.SelfMCPServerEntry())
		})
	}
}

// A project that serves skills runs `mcp --serve-skills` from the self entry, so
// one .mcp.json entry serves both the config tools and the skills.
func TestSelfMCPServerEntry_ServesSkillsFromTheSameEntry(t *testing.T) {
	serving := []SkillSourceConfig{{Name: "vendor", URL: "vendor-skills"}}
	tests := []struct {
		name string
		mcp  *MCPConfig
		want map[string]any
	}{
		{
			name: "the default launch gains --serve-skills",
			mcp:  &MCPConfig{SelfServer: boolPtr(true)},
			want: map[string]any{"type": "stdio", "command": "npx", "args": []string{"-y", "ai-rulez@latest", "mcp", "--serve-skills"}},
		},
		{
			name: "a version pin keeps the flag",
			mcp:  &MCPConfig{SelfServer: boolPtr(true), SelfServerVersion: "4.1.0"},
			want: map[string]any{"type": "stdio", "command": "npx", "args": []string{"-y", "ai-rulez@4.1.0", "mcp", "--serve-skills"}},
		},
		{
			name: "a custom command gains the flag too",
			mcp:  &MCPConfig{SelfServer: boolPtr(true), SelfServerCommand: []string{"ai-rulez", "mcp"}},
			want: map[string]any{"type": "stdio", "command": "ai-rulez", "args": []string{"mcp", "--serve-skills"}},
		},
		{
			name: "a command that already serves is left as it is",
			mcp:  &MCPConfig{SelfServer: boolPtr(true), SelfServerCommand: []string{"ai-rulez", "mcp", "--serve-skills"}},
			want: map[string]any{"type": "stdio", "command": "ai-rulez", "args": []string{"mcp", "--serve-skills"}},
		},
		{
			name: "a shell script launch never gets the flag appended as $0",
			mcp:  &MCPConfig{SelfServer: boolPtr(true), SelfServerCommand: []string{"sh", "-c", `exec ai-rulez mcp --serve-skills "$@"`, "sh"}},
			want: map[string]any{"type": "stdio", "command": "sh", "args": []string{"-c", `exec ai-rulez mcp --serve-skills "$@"`, "sh"}},
		},
		{
			name: "binary-then-npx passes the flag as a script argument",
			mcp:  &MCPConfig{SelfServer: boolPtr(true), SelfServerLauncher: SelfLauncherBinaryThenNPX, SelfServerVersion: "4.1.0"},
			want: map[string]any{"type": "stdio", "command": "sh", "args": []string{"-c", fmt.Sprintf(binaryThenNPXScript, "4.1.0"), "sh", "--serve-skills"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{MCP: tt.mcp, SkillSources: serving}
			assert.Equal(t, tt.want, cfg.SelfMCPServerEntry())
		})
	}

	// Without served skills the entry stays the plain authoring launch.
	plain := &Config{MCP: &MCPConfig{SelfServer: boolPtr(true)}}
	assert.Equal(t,
		map[string]any{"type": "stdio", "command": "npx", "args": []string{"-y", "ai-rulez@latest", "mcp"}},
		plain.SelfMCPServerEntry())
}

// The self server is on out of the box: only an explicit false turns it off.
func TestHasSelfServerDefaultsOn(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want bool
	}{
		{name: "nil config", cfg: nil, want: false},
		{name: "no [mcp] table", cfg: &Config{}, want: true},
		{name: "empty [mcp] table", cfg: &Config{MCP: &MCPConfig{}}, want: true},
		{name: "self_server unset", cfg: &Config{MCP: &MCPConfig{SelfServerVersion: "5.0.0"}}, want: true},
		{name: "explicit true", cfg: &Config{MCP: &MCPConfig{SelfServer: boolPtr(true)}}, want: true},
		{name: "explicit false", cfg: &Config{MCP: &MCPConfig{SelfServer: boolPtr(false)}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.HasSelfServer())
		})
	}
}

func TestSetSelfServer(t *testing.T) {
	// Forces off even when the default would be on, and creates the table.
	cfg := &Config{}
	cfg.SetSelfServer(false)
	require.NotNil(t, cfg.MCP)
	assert.False(t, cfg.HasSelfServer())

	// Forces back on.
	cfg.SetSelfServer(true)
	assert.True(t, cfg.HasSelfServer())
}

func TestConfigValidateMCP(t *testing.T) {
	tests := []struct {
		name    string
		mcp     *MCPConfig
		wantErr string
	}{
		{name: "absent", mcp: nil},
		{name: "self server only", mcp: &MCPConfig{SelfServer: boolPtr(true)}},
		{name: "version pin", mcp: &MCPConfig{SelfServer: boolPtr(true), SelfServerVersion: "4.19.0"}},
		{name: "dist tag", mcp: &MCPConfig{SelfServer: boolPtr(true), SelfServerVersion: "latest"}},
		{name: "command override", mcp: &MCPConfig{SelfServer: boolPtr(true), SelfServerCommand: []string{"ai-rulez", "mcp"}}},
		{name: "version pin with the default on", mcp: &MCPConfig{SelfServerVersion: "4.19.0"}},
		{name: "explicitly off", mcp: &MCPConfig{SelfServer: boolPtr(false)}},
		{name: "version while off", mcp: &MCPConfig{SelfServer: boolPtr(false), SelfServerVersion: "4.19.0"}, wantErr: "require mcp.self_server = true"},
		{name: "command while off", mcp: &MCPConfig{SelfServer: boolPtr(false), SelfServerCommand: []string{"ai-rulez"}}, wantErr: "require mcp.self_server = true"},
		{name: "empty executable", mcp: &MCPConfig{SelfServer: boolPtr(true), SelfServerCommand: []string{" ", "mcp"}}, wantErr: "executable must not be empty"},
		{name: "version and command", mcp: &MCPConfig{SelfServer: boolPtr(true), SelfServerVersion: "1.0.0", SelfServerCommand: []string{"x"}}, wantErr: "mutually exclusive"},
		{name: "version with at sign", mcp: &MCPConfig{SelfServer: boolPtr(true), SelfServerVersion: "ai-rulez@1.0.0"}, wantErr: "not a valid version"},
		{name: "launcher binary-then-npx", mcp: &MCPConfig{SelfServer: boolPtr(true), SelfServerLauncher: "binary-then-npx"}},
		{name: "launcher unknown", mcp: &MCPConfig{SelfServer: boolPtr(true), SelfServerLauncher: "docker"}, wantErr: "invalid mcp.self_server_launcher"},
		{name: "launcher and command", mcp: &MCPConfig{SelfServer: boolPtr(true), SelfServerLauncher: "binary-then-npx", SelfServerCommand: []string{"x"}}, wantErr: "mutually exclusive"},
		{name: "version with space", mcp: &MCPConfig{SelfServer: boolPtr(true), SelfServerVersion: "1.0 0"}, wantErr: "not a valid version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Version: "5.0", Name: "test", Presets: []Preset{{BuiltIn: "claude"}}, MCP: tt.mcp}
			err := cfg.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLoadConfigTOML_MCPSection(t *testing.T) {
	// Absent [mcp] defaults the self server on.
	off := writeTOMLProject(t, `version = "5.0"
name = "proj"
presets = ["claude"]
`)
	cfgOff, err := LoadConfig(context.Background(), off)
	require.NoError(t, err)
	assert.True(t, cfgOff.HasSelfServer(), "the self server is on by default")

	// An explicit false turns it off, and the value round-trips.
	baseDir := writeTOMLProject(t, `version = "5.0"
name = "proj"
presets = ["claude"]

[mcp]
self_server = false
`)
	cfg, err := LoadConfig(context.Background(), baseDir)
	require.NoError(t, err)
	require.NotNil(t, cfg.MCP)
	assert.False(t, cfg.HasSelfServer())

	on := writeTOMLProject(t, `version = "5.0"
name = "proj"
presets = ["claude"]

[mcp]
self_server = true
self_server_version = "4.19.0"
`)
	cfgOn, err := LoadConfig(context.Background(), on)
	require.NoError(t, err)
	require.NotNil(t, cfgOn.MCP)
	assert.True(t, cfgOn.HasSelfServer())
	assert.Equal(t, "4.19.0", cfgOn.MCP.SelfServerVersion)

	out, err := MarshalTOML(cfgOn)
	require.NoError(t, err)
	assert.Contains(t, string(out), "self_server = true")
}

func TestHasSelfServer_PluginAuthoringRoot(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want bool
	}{
		{"project root", &Config{Presets: []Preset{{BuiltIn: "claude"}}}, true},
		{"project root that also authors a plugin", &Config{Presets: []Preset{{BuiltIn: "claude"}}, Plugin: &PluginAuthoring{}}, true},
		{"plugin-only root", &Config{Plugin: &PluginAuthoring{}}, false},
		{"plugin-only root, self_server forced on", &Config{Plugin: &PluginAuthoring{}, MCP: &MCPConfig{SelfServer: boolPtr(true)}}, false},
		{"plugin definitions only", &Config{PluginsAuthoring: []MarketplacePlugin{{}}}, false},
		{"lint-only root", &Config{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := tt.cfg.HasSelfServer()

			// Assert
			assert.Equal(t, tt.want, got)
			assert.Equal(t, !tt.want && tt.cfg.HasPluginAuthoring(), tt.cfg.IsPluginAuthoringRoot())
		})
	}
}
