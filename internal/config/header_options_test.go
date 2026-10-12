package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHeaderMCPCommand(t *testing.T) {
	serving := []SkillSourceConfig{{Name: "vendor", URL: "vendor-skills"}}
	off := false
	tests := []struct {
		name     string
		cfg      *Config
		wantCmd  string
		wantShow bool
	}{
		{"default launch", &Config{}, "npx -y ai-rulez@latest mcp", true},
		{"pinned version", &Config{MCP: &MCPConfig{SelfServerVersion: "4.1.0"}}, "npx -y ai-rulez@4.1.0 mcp", true},
		{"custom command", &Config{MCP: &MCPConfig{SelfServerCommand: []string{"ai-rulez", "mcp"}}}, "ai-rulez mcp", true},
		{
			"custom command serving skills",
			&Config{MCP: &MCPConfig{SelfServerCommand: []string{"ai-rulez", "mcp"}}, SkillSources: serving},
			"ai-rulez mcp --serve-skills", true,
		},
		{"default launch serving skills", &Config{SkillSources: serving}, "npx -y ai-rulez@latest mcp --serve-skills", true},
		{"self server off", &Config{MCP: &MCPConfig{SelfServer: &off}}, "", false},
		{"mcp_line off", &Config{Header: &HeaderConfig{MCPLine: &off}}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, show := tt.cfg.HeaderMCPCommand()
			assert.Equal(t, tt.wantShow, show)
			assert.Equal(t, tt.wantCmd, cmd)
		})
	}
}

func TestNestedHeaderMode(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want string
	}{
		{"root defaults to inherit", &Config{}, HeaderNestedInherit},
		{"nested defaults to compact", &Config{NestedRoot: true}, HeaderNestedCompact},
		{"nested omit", &Config{NestedRoot: true, Header: &HeaderConfig{Nested: HeaderNestedOmit}}, HeaderNestedOmit},
		{"nested inherit", &Config{NestedRoot: true, Header: &HeaderConfig{Nested: HeaderNestedInherit}}, HeaderNestedInherit},
		{"root ignores omit", &Config{Header: &HeaderConfig{Nested: HeaderNestedOmit}}, HeaderNestedInherit},
		{"custom text keeps an unset nested mode", &Config{NestedRoot: true, Header: &HeaderConfig{Text: "mine"}}, HeaderNestedInherit},
		{"custom text yields to an explicit mode", &Config{NestedRoot: true, Header: &HeaderConfig{Text: "mine", Nested: HeaderNestedOmit}}, HeaderNestedOmit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.NestedHeaderMode())
		})
	}
}

func TestValidateHeaderNested(t *testing.T) {
	cfg := &Config{Name: "p", Header: &HeaderConfig{Nested: "bogus"}}
	err := cfg.validateHeaderNested()
	assert.ErrorContains(t, err, "header.nested")
	for _, ok := range []string{"", HeaderNestedCompact, HeaderNestedOmit, HeaderNestedInherit} {
		cfg.Header.Nested = ok
		assert.NoError(t, cfg.validateHeaderNested(), ok)
	}
}
