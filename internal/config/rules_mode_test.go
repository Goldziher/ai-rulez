package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRulesModeFor(t *testing.T) {
	tests := []struct {
		name   string
		cfg    *Config
		preset string
		want   string
	}{
		{"nil config", nil, "claude", defaultRulesMode},
		{"no rules section", &Config{}, "claude", defaultRulesMode},
		{"empty rules table", &Config{Rules: &RulesConfig{}}, "claude", defaultRulesMode},
		{"global mode", &Config{Rules: &RulesConfig{Mode: RulesModeSplit}}, "claude", RulesModeSplit},
		{
			"by-preset beats global",
			&Config{Rules: &RulesConfig{Mode: RulesModeSplit, ModeByPreset: map[string]string{"claude": RulesModeInline}}},
			"claude", RulesModeInline,
		},
		{
			"other preset falls back to global",
			&Config{Rules: &RulesConfig{Mode: RulesModeSplit, ModeByPreset: map[string]string{"claude": RulesModeInline}}},
			"cursor", RulesModeSplit,
		},
		{
			"by-preset without global",
			&Config{Rules: &RulesConfig{ModeByPreset: map[string]string{"cursor": RulesModeSplit}}},
			"cursor", RulesModeSplit,
		},
		{
			"empty by-preset value ignored",
			&Config{Rules: &RulesConfig{Mode: RulesModeSplit, ModeByPreset: map[string]string{"cursor": ""}}},
			"cursor", RulesModeSplit,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.RulesModeFor(tt.preset))
		})
	}
}

func TestRulesModeExplicitFor(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want bool
	}{
		{"nil config", nil, false},
		{"no rules section", &Config{}, false},
		{"global mode only", &Config{Rules: &RulesConfig{Mode: RulesModeSplit}}, false},
		{"by-preset set", &Config{Rules: &RulesConfig{ModeByPreset: map[string]string{"claude": RulesModeSplit}}}, true},
		{"by-preset other preset", &Config{Rules: &RulesConfig{ModeByPreset: map[string]string{"cursor": RulesModeSplit}}}, false},
		{"by-preset empty value", &Config{Rules: &RulesConfig{ModeByPreset: map[string]string{"claude": ""}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.RulesModeExplicitFor("claude"))
		})
	}
}

func TestValidate_RulesMode(t *testing.T) {
	tests := []struct {
		name    string
		rules   *RulesConfig
		wantErr string
	}{
		{"nil rules", nil, ""},
		{"valid split", &RulesConfig{Mode: "split"}, ""},
		{"valid inline", &RulesConfig{Mode: "inline"}, ""},
		{"empty rules table is treated as unset", &RulesConfig{}, ""},
		{"unknown mode", &RulesConfig{Mode: "both"}, `invalid rules mode "both"`},
		{"case sensitive", &RulesConfig{Mode: "Split"}, `invalid rules mode "Split"`},
		{"valid by-preset", &RulesConfig{ModeByPreset: map[string]string{"claude": "split", "cursor": "inline"}}, ""},
		{"custom preset key", &RulesConfig{ModeByPreset: map[string]string{"docs": "inline"}}, ""},
		{"unknown preset key", &RulesConfig{ModeByPreset: map[string]string{"nope": "split"}}, `unknown preset "nope"`},
		{"unknown by-preset value", &RulesConfig{ModeByPreset: map[string]string{"claude": "x"}}, `invalid rules mode "x"`},
		{"empty by-preset value", &RulesConfig{ModeByPreset: map[string]string{"claude": ""}}, "empty rules mode"},
		{"baz nested", &RulesConfig{BazScoped: "nested"}, ""},
		{"baz root", &RulesConfig{BazScoped: "root"}, ""},
		{"baz unknown", &RulesConfig{BazScoped: "deep"}, `invalid baz_scoped value "deep"`},
		{"baz case sensitive", &RulesConfig{BazScoped: "Root"}, `invalid baz_scoped value "Root"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Version: "4.0",
				Name:    "test",
				Presets: []Preset{{BuiltIn: "claude"}, {Name: "docs", Type: PresetTypeMarkdown, Path: "DOCS.md"}},
				Rules:   tt.rules,
			}
			err := cfg.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestTOMLWriter_RulesRoundTrip(t *testing.T) {
	baseDir := writeTOMLProject(t, `version = "4.0"
name = "proj"
presets = ["claude", "cursor"]

[rules]
mode = "split"

[rules.mode_by_preset]
cursor = "inline"
`)
	cfg, err := LoadConfig(context.Background(), baseDir)
	require.NoError(t, err)
	require.NotNil(t, cfg.Rules)
	assert.Equal(t, "split", cfg.Rules.Mode)
	assert.Equal(t, map[string]string{"cursor": "inline"}, cfg.Rules.ModeByPreset)

	data, err := MarshalTOML(cfg)
	require.NoError(t, err)

	reloadDir := writeTOMLProject(t, string(data))
	reloaded, err := LoadConfig(context.Background(), reloadDir)
	require.NoError(t, err)
	assert.Equal(t, cfg.Rules, reloaded.Rules)
}

func TestLoadConfig_RulesTOML(t *testing.T) {
	want := &RulesConfig{Mode: "split", ModeByPreset: map[string]string{"cursor": "inline"}}
	tests := []struct {
		name string
		file string
		body string
	}{
		{
			"toml", "config.toml",
			"version = \"4.0\"\nname = \"proj\"\npresets = [\"claude\", \"cursor\"]\n\n[rules]\nmode = \"split\"\n\n[rules.mode_by_preset]\ncursor = \"inline\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := t.TempDir()
			configDir := filepath.Join(baseDir, aiRulezDirName)
			require.NoError(t, os.MkdirAll(configDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(configDir, tt.file), []byte(tt.body), 0o644))

			cfg, err := LoadConfig(context.Background(), baseDir)
			require.NoError(t, err)
			assert.Equal(t, want, cfg.Rules)
			assert.Equal(t, "inline", cfg.RulesModeFor("cursor"))
			assert.Equal(t, "split", cfg.RulesModeFor("claude"))
		})
	}
}

func TestTOMLWriter_OmitsEmptyRules(t *testing.T) {
	data, err := MarshalTOML(&Config{Version: "4.0", Name: "p", Presets: []Preset{{BuiltIn: "claude"}}})
	require.NoError(t, err)
	assert.NotContains(t, string(data), "[rules]")
}

func TestBazScopedRules(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want string
	}{
		{"nil config", nil, BazScopedNested},
		{"no rules block", &Config{}, BazScopedNested},
		{"empty rules block", &Config{Rules: &RulesConfig{}}, BazScopedNested},
		{"root", &Config{Rules: &RulesConfig{BazScoped: "root"}}, BazScopedRoot},
		{"nested", &Config{Rules: &RulesConfig{BazScoped: "nested"}}, BazScopedNested},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.BazScopedRules())
		})
	}
}

func TestHasBuiltInPreset(t *testing.T) {
	cfg := &Config{Presets: []Preset{{BuiltIn: "claude"}, {Name: "baz", Type: PresetTypeMarkdown, Path: "X.md"}}}
	assert.True(t, cfg.HasBuiltInPreset("claude"))
	assert.False(t, cfg.HasBuiltInPreset("baz"), "a custom preset named baz is not the built-in")
	assert.False(t, (*Config)(nil).HasBuiltInPreset("claude"))
}
