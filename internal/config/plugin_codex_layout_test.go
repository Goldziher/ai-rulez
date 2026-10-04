package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCodexManifestValidation(t *testing.T) {
	t.Parallel()
	base := func(codex *CodexExtras, name string, runtimes ...string) *Config {
		return &Config{Plugin: &PluginAuthoring{Name: name, Version: "1.0.0", Description: "d", Author: &Author{Name: "a"}, Interface: &PluginInterface{DisplayName: "d", ShortDescription: "s", LongDescription: "l", DeveloperName: "dev", Category: "c", Capabilities: []string{"x"}, DefaultPrompt: []string{"p"}}, Runtimes: runtimes, Codex: codex}}
	}
	tests := []struct {
		name    string
		cfg     *Config
		wantErr string
	}{
		{"default layout", base(nil, "my-plugin", "codex"), ""},
		{"root layout", base(&CodexExtras{Manifest: "root"}, "my-plugin", "codex"), ""},
		{"unknown layout", base(&CodexExtras{Manifest: "sideways"}, "my-plugin", "codex"), "unknown codex manifest layout"},
		{"root layout needs an Agent Plugins name", base(&CodexExtras{Manifest: "root"}, "My_Plugin", "codex"), "not valid for the agent-plugins runtime"},
		{"legacy layout keeps the old name rules", base(nil, "my-plugin", "codex"), ""},
		{"copilot needs an Agent Plugins name", base(nil, "My_Plugin", "copilot"), "not valid for the agent-plugins runtime"},
		{"copilot is a known runtime", base(nil, "my-plugin", "copilot"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.validatePluginAuthoring()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			if assert.Error(t, err) {
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}
