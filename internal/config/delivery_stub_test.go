package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stubOf(cfg *Config) ContentFile { return cfg.dynamicSkillsStub() }

func TestDynamicSkillsStub_DefaultsAreUnchanged(t *testing.T) {
	stub := stubOf(&Config{})
	assert.Equal(t, DynamicSkillsStub(DefaultSkillsServerName).Content, stub.Content)
	assert.Equal(t, dynamicSkillsDescription, stub.Metadata.Extra[keyFrontmatterDescription])
}

func TestDynamicSkillsStub_Customised(t *testing.T) {
	cfg := &Config{
		MCPServersRaw: []MCPServer{{Name: "skills", Command: "ai-rulez", Args: []string{"mcp", "--serve-skills"}}},
		Delivery: &DeliveryConfig{Stub: &StubConfig{
			Description: "Find team skills.",
			Body:        "Ask {{server}} with find_skill, then load_skill.\n",
		}},
	}
	stub := stubOf(cfg)
	assert.Equal(t, "Ask skills with find_skill, then load_skill.\n", stub.Content)
	assert.Equal(t, "Find team skills.", stub.Metadata.Extra[keyFrontmatterDescription])
	assert.Equal(t, DynamicSkillsName, stub.Name)
}

func TestDynamicSkillsStub_DescriptionOnlyKeepsTheDefaultBody(t *testing.T) {
	cfg := &Config{Delivery: &DeliveryConfig{Stub: &StubConfig{Description: "Short."}}}
	stub := stubOf(cfg)
	assert.Equal(t, DynamicSkillsStub(DefaultSkillsServerName).Content, stub.Content)
	assert.Equal(t, "Short.", stub.Metadata.Extra[keyFrontmatterDescription])
}

func TestContentForPreset_UsesTheCustomStub(t *testing.T) {
	cfg := &Config{
		DomainSettings: map[string]DomainConfig{"billing": {Delivery: "served"}},
		Content:        treeWithSkills(),
		Delivery:       &DeliveryConfig{Stub: &StubConfig{Description: "Custom.", Body: "Use {{server}}: find_skill / load_skill"}},
	}
	for _, s := range cfg.ContentForPreset("claude").Skills {
		if SkillID(s) == DynamicSkillsName {
			assert.Equal(t, "Use ai-rulez: find_skill / load_skill", s.Content)
			assert.Equal(t, "Custom.", s.Metadata.Extra[keyFrontmatterDescription])
			return
		}
	}
	t.Fatal("no stub rendered")
}

func TestLoadConfig_StubBodyFile(t *testing.T) {
	base := writeTOMLProject(t, `version = "5.0"
name = "proj"
presets = ["claude"]

[delivery.stub]
description = "From file."
body_file = "stub.md"
`)
	require.NoError(t, os.WriteFile(filepath.Join(base, aiRulezDirName, "stub.md"), []byte("Call find_skill on {{server}}, then load_skill.\n"), 0o644))

	cfg, err := LoadConfig(context.Background(), base)
	require.NoError(t, err)
	require.NoError(t, cfg.validateDeliveryStub())
	stub := stubOf(cfg)
	assert.Equal(t, "Call find_skill on ai-rulez, then load_skill.\n", stub.Content)
	assert.Equal(t, "From file.", stub.Metadata.Extra[keyFrontmatterDescription])
}

func TestLoadConfig_StubBodyFileMissingOrOutsideIsRefused(t *testing.T) {
	for name, file := range map[string]string{"missing": "nope.md", "escaping": "../outside.md", "absolute": "/etc/hosts"} {
		t.Run(name, func(t *testing.T) {
			base := writeTOMLProject(t, "version = \"5.0\"\nname = \"proj\"\npresets = [\"claude\"]\n\n[delivery.stub]\nbody_file = \""+file+"\"\n")
			_, err := LoadConfig(context.Background(), base)
			assert.ErrorContains(t, err, "delivery.stub.body_file")
		})
	}
}

func TestValidateDeliveryStub(t *testing.T) {
	const ok = "Use find_skill then load_skill on {{server}}."
	tests := []struct {
		name    string
		stub    *StubConfig
		wantErr string
	}{
		{"unset", nil, ""},
		{"description only", &StubConfig{Description: "x"}, ""},
		{"valid body", &StubConfig{Body: ok}, ""},
		{"blank description", &StubConfig{Description: "  ", Body: ok}, "description must not be blank"},
		{"body and file", &StubConfig{Body: ok, BodyFile: "a.md"}, "mutually exclusive"},
		{"unknown variable", &StubConfig{Body: "find_skill load_skill {{srv}}"}, "unknown variable {{srv}}"},
		{"names neither tool", &StubConfig{Body: "Skills exist."}, "find_skill"},
		{"names only one tool", &StubConfig{Body: "Call find_skill."}, "load_skill"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			if tt.stub != nil {
				cfg.Delivery = &DeliveryConfig{Stub: tt.stub}
			}
			err := cfg.validateDeliveryStub()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}
