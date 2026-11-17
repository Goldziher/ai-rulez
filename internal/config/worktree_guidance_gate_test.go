package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorktreeGuidanceToolGate(t *testing.T) {
	on := true
	off := false
	tests := []struct {
		name       string
		mcp        *MCPConfig
		preset     bool
		namesTools bool
	}{
		{"default server exposes it", nil, true, true},
		{"no mcp preset: the sentence self-qualifies, so the wording stays", nil, false, true},
		{"read-only server does not", &MCPConfig{ReadOnly: &on}, true, false},
		{"allowlist without the tool does not", &MCPConfig{Tools: []string{"read_rule"}}, true, false},
		{"allowlist with the tool does", &MCPConfig{Tools: []string{"read_rule", "worktree_setup"}}, true, true},
		{"explicitly writable", &MCPConfig{ReadOnly: &off}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Version: "5.0", Name: "t", MCP: tt.mcp}
			if tt.preset {
				cfg.Presets = []Preset{{BuiltIn: "claude"}}
			}
			text, show := cfg.WorktreeGuidance()
			assert.True(t, show, "the guidance itself always renders; the gate decides the wording")
			assert.Equal(t, tt.namesTools, strings.Contains(text, "worktree_setup"))
		})
	}
}
