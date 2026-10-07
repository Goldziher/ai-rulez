package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelfMCPServerEntry(t *testing.T) {
	tests := []struct {
		name          string
		mcp           *MCPConfig
		binaryVersion string
		want          map[string]any
	}{
		{
			name:          "defaults to the binary version",
			mcp:           &MCPConfig{SelfServer: true},
			binaryVersion: "4.19.0",
			want:          map[string]any{"type": "stdio", "command": "npx", "args": []string{"-y", "ai-rulez@4.19.0", "mcp"}},
		},
		{
			name:          "dev build resolves to latest",
			mcp:           &MCPConfig{SelfServer: true},
			binaryVersion: "dev",
			want:          map[string]any{"type": "stdio", "command": "npx", "args": []string{"-y", "ai-rulez@latest", "mcp"}},
		},
		{
			name:          "empty binary version resolves to latest",
			mcp:           &MCPConfig{SelfServer: true},
			binaryVersion: "",
			want:          map[string]any{"type": "stdio", "command": "npx", "args": []string{"-y", "ai-rulez@latest", "mcp"}},
		},
		{
			name:          "configured version wins over the binary",
			mcp:           &MCPConfig{SelfServer: true, SelfServerVersion: "4.1.0"},
			binaryVersion: "4.19.0",
			want:          map[string]any{"type": "stdio", "command": "npx", "args": []string{"-y", "ai-rulez@4.1.0", "mcp"}},
		},
		{
			name:          "command override replaces npx entirely",
			mcp:           &MCPConfig{SelfServer: true, SelfServerCommand: []string{"ai-rulez", "mcp"}},
			binaryVersion: "4.19.0",
			want:          map[string]any{"type": "stdio", "command": "ai-rulez", "args": []string{"mcp"}},
		},
		{
			name:          "command override without arguments omits args",
			mcp:           &MCPConfig{SelfServer: true, SelfServerCommand: []string{"ai-rulez-mcp"}},
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

func TestConfigValidateMCP(t *testing.T) {
	tests := []struct {
		name    string
		mcp     *MCPConfig
		wantErr string
	}{
		{name: "absent", mcp: nil},
		{name: "self server only", mcp: &MCPConfig{SelfServer: true}},
		{name: "version pin", mcp: &MCPConfig{SelfServer: true, SelfServerVersion: "4.19.0"}},
		{name: "dist tag", mcp: &MCPConfig{SelfServer: true, SelfServerVersion: "latest"}},
		{name: "command override", mcp: &MCPConfig{SelfServer: true, SelfServerCommand: []string{"ai-rulez", "mcp"}}},
		{name: "version without self_server", mcp: &MCPConfig{SelfServerVersion: "4.19.0"}, wantErr: "require mcp.self_server = true"},
		{name: "command without self_server", mcp: &MCPConfig{SelfServerCommand: []string{"ai-rulez"}}, wantErr: "require mcp.self_server = true"},
		{name: "empty executable", mcp: &MCPConfig{SelfServer: true, SelfServerCommand: []string{" ", "mcp"}}, wantErr: "executable must not be empty"},
		{name: "version and command", mcp: &MCPConfig{SelfServer: true, SelfServerVersion: "1.0.0", SelfServerCommand: []string{"x"}}, wantErr: "mutually exclusive"},
		{name: "version with at sign", mcp: &MCPConfig{SelfServer: true, SelfServerVersion: "ai-rulez@1.0.0"}, wantErr: "not a valid version"},
		{name: "version with space", mcp: &MCPConfig{SelfServer: true, SelfServerVersion: "1.0 0"}, wantErr: "not a valid version"},
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
	baseDir := writeTOMLProject(t, `version = "5.0"
name = "proj"
presets = ["claude"]

[mcp]
self_server = true
self_server_version = "4.19.0"
`)
	cfg, err := LoadConfig(context.Background(), baseDir)
	require.NoError(t, err)
	require.NotNil(t, cfg.MCP)
	assert.True(t, cfg.HasSelfServer())
	assert.Equal(t, "4.19.0", cfg.MCP.SelfServerVersion)

	out, err := MarshalTOML(cfg)
	require.NoError(t, err)
	assert.Contains(t, string(out), "self_server = true")
}
