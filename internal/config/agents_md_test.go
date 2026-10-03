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
		{preset: "cursor"},
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
