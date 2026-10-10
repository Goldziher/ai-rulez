package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }

func TestSelfMCPServerEntry(t *testing.T) {
	tests := []struct {
		name          string
		mcp           *MCPConfig
		binaryVersion string
		want          map[string]any
	}{
		{
			name:          "defaults to the binary version",
			mcp:           &MCPConfig{SelfServer: boolPtr(true)},
			binaryVersion: "4.19.0",
			want:          map[string]any{"type": "stdio", "command": "npx", "args": []string{"-y", "ai-rulez@4.19.0", "mcp"}},
		},
		{
			name:          "dev build resolves to latest",
			mcp:           &MCPConfig{SelfServer: boolPtr(true)},
			binaryVersion: "dev",
			want:          map[string]any{"type": "stdio", "command": "npx", "args": []string{"-y", "ai-rulez@latest", "mcp"}},
		},
		{
			name:          "empty binary version resolves to latest",
			mcp:           &MCPConfig{SelfServer: boolPtr(true)},
			binaryVersion: "",
			want:          map[string]any{"type": "stdio", "command": "npx", "args": []string{"-y", "ai-rulez@latest", "mcp"}},
		},
		{
			name:          "configured version wins over the binary",
			mcp:           &MCPConfig{SelfServer: boolPtr(true), SelfServerVersion: "4.1.0"},
			binaryVersion: "4.19.0",
			want:          map[string]any{"type": "stdio", "command": "npx", "args": []string{"-y", "ai-rulez@4.1.0", "mcp"}},
		},
		{
			name:          "command override replaces npx entirely",
			mcp:           &MCPConfig{SelfServer: boolPtr(true), SelfServerCommand: []string{"ai-rulez", "mcp"}},
			binaryVersion: "4.19.0",
			want:          map[string]any{"type": "stdio", "command": "ai-rulez", "args": []string{"mcp"}},
		},
		{
			name:          "command override without arguments omits args",
			mcp:           &MCPConfig{SelfServer: boolPtr(true), SelfServerCommand: []string{"ai-rulez-mcp"}},
			binaryVersion: "4.19.0",
			want:          map[string]any{"type": "stdio", "command": "ai-rulez-mcp"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{MCP: tt.mcp}
			assert.Equal(t, tt.want, cfg.SelfMCPServerEntry(tt.binaryVersion))
		})
	}
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
