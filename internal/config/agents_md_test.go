package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentsMD_LoadAndSaveRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"absent", "version = \"4.0\"\nname = \"p\"\npresets = [\"codex\"]\n", false},
		{"false", "version = \"4.0\"\nname = \"p\"\nagents_md = false\npresets = [\"codex\"]\n", false},
		{"true", "version = \"4.0\"\nname = \"p\"\nagents_md = true\npresets = [\"codex\"]\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseDir := writeTOMLProject(t, tc.body)
			cfg, err := LoadConfig(context.Background(), baseDir)
			require.NoError(t, err)
			assert.Equal(t, tc.want, cfg.AgentsMD)

			require.NoError(t, SaveConfig(cfg, cfg.ConfigDir))
			reloaded, err := LoadConfig(context.Background(), baseDir)
			require.NoError(t, err)
			assert.Equal(t, tc.want, reloaded.AgentsMD)
		})
	}
}

func TestSharedOutputConsumerFor(t *testing.T) {
	cases := []struct {
		preset      string
		ok          bool
		agentsMD    bool // reads AGENTS.md directly
		skills      bool // reads .agents/skills
		imports     bool // imports AGENTS.md from its own root file
		ownSkill    string
		ownRootFile string
	}{
		{preset: "codex", ok: true, agentsMD: true, skills: true, ownSkill: ".codex/skills"},
		{preset: "opencode", ok: true, agentsMD: true, skills: true, ownSkill: ".opencode/skills"},
		{preset: "xum", ok: true, agentsMD: true, skills: true, ownSkill: ".xum/skills"},
		{preset: "amp", ok: true, agentsMD: true, skills: true},
		{preset: "claude", ok: true, imports: true},
		{preset: "gemini", ok: true, agentsMD: true, skills: true, ownRootFile: "GEMINI.md"},
		{preset: "antigravity", ok: true, agentsMD: true, skills: true, ownRootFile: "GEMINI.md"},
		{preset: "hermes", ok: true, agentsMD: true, skills: true, ownRootFile: ".hermes.md"},
		{preset: "cursor", ok: true, agentsMD: true, skills: true},
		{preset: "copilot", ok: true, agentsMD: true, skills: true, ownSkill: ".github/skills", ownRootFile: ".github/copilot-instructions.md"},
		{preset: "junie", ok: true, agentsMD: true, skills: true, ownSkill: ".junie/skills"},
		{preset: "devin", ok: true, agentsMD: true, skills: true, ownSkill: ".devin/skills"},
		{preset: "cline", ok: true, agentsMD: true, skills: true, ownSkill: ".cline/skills"},
		{preset: "mcp"},
	}
	for _, tc := range cases {
		t.Run(tc.preset, func(t *testing.T) {
			consumer, ok := SharedOutputConsumerFor(tc.preset)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.ownSkill, consumer.OwnSkillsDir)
			assert.Equal(t, tc.ownRootFile, consumer.OwnRootFile)
			assert.Equal(t, tc.agentsMD, consumer.Reads(SharedAgentsMD))
			assert.Equal(t, tc.skills, consumer.Reads(SharedAgentSkills))
			assert.Equal(t, tc.imports, consumer.ImportsAgentsMD)
			assert.Equal(t, tc.agentsMD || tc.imports, consumer.NeedsAgentsMD())
		})
	}
}

func TestSharedOutputConsumerFor_RootFiles(t *testing.T) {
	cases := map[string]string{
		"claude": "CLAUDE.md", "gemini": "GEMINI.md", "antigravity": "GEMINI.md", "hermes": ".hermes.md",
		"copilot": ".github/copilot-instructions.md", "junie": "", "codex": "", "cursor": "",
	}
	for preset, want := range cases {
		t.Run(preset, func(t *testing.T) {
			consumer, ok := SharedOutputConsumerFor(preset)
			require.True(t, ok)
			assert.Equal(t, want, consumer.ReplacedRootFile())
			assert.Equal(t, want != "", consumer.HasRootFile())
		})
	}
}

func TestSharedAgentsMDInlining(t *testing.T) {
	cases := []struct {
		name    string
		presets []string
		rules   *RulesConfig
		want    AgentsMDInlining
	}{
		{"no consumer", []string{"mcp"}, nil, AgentsMDInlining{}},
		{"folder presets only", []string{"claude", "cursor", "devin", "cline", "junie", "antigravity"}, nil, AgentsMDInlining{}},
		{"copilot adds auto and manual", []string{"copilot", "cursor"}, nil, AgentsMDInlining{AutoManual: true}},
		{"codex has no folder", []string{"claude", "codex"}, nil, AgentsMDInlining{Scoped: true, AutoManual: true}},
		{"gemini has no folder", []string{"cursor", "gemini"}, nil, AgentsMDInlining{Scoped: true, AutoManual: true}},
		{"hermes has no folder", []string{"hermes"}, nil, AgentsMDInlining{Scoped: true, AutoManual: true}},
		{"claude in inline mode", []string{"claude"}, &RulesConfig{Mode: RulesModeInline}, AgentsMDInlining{AutoManual: true}},
		{"antigravity in inline mode", []string{"antigravity"}, &RulesConfig{Mode: RulesModeInline}, AgentsMDInlining{AutoManual: true}},
		{"junie in inline mode alone", []string{"junie"}, &RulesConfig{Mode: RulesModeInline}, AgentsMDInlining{Scoped: true, AutoManual: true}},
		{"copilot in inline mode", []string{"copilot"}, &RulesConfig{Mode: RulesModeInline}, AgentsMDInlining{AutoManual: true}},
		{"claude split and junie inline", []string{"claude", "junie"}, &RulesConfig{ModeByPreset: map[string]string{"junie": RulesModeInline}}, AgentsMDInlining{Scoped: true, AutoManual: true}},
		{"junie in inline mode", []string{"cursor", "junie"}, &RulesConfig{ModeByPreset: map[string]string{"junie": RulesModeInline}}, AgentsMDInlining{Scoped: true, AutoManual: true}},
		{"cursor ignores inline mode", []string{"cursor"}, &RulesConfig{Mode: RulesModeInline}, AgentsMDInlining{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{AgentsMD: true, Rules: tc.rules}
			for _, p := range tc.presets {
				cfg.Presets = append(cfg.Presets, Preset{BuiltIn: p})
			}
			assert.Equal(t, tc.want, SharedAgentsMDInlining(cfg))
		})
	}
}
