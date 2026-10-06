package telemetry

import (
	"encoding/json"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHookTemplateGoldens(t *testing.T) {
	jsonOut, err := HookTemplate(TemplateOptions{})
	require.NoError(t, err)
	assert.True(t, json.Valid(jsonOut))
	golden(t, "hook_claude.golden.json", jsonOut)

	tomlOut, err := HookTemplate(TemplateOptions{Format: FormatTOML, Role: "backend"})
	require.NoError(t, err)
	golden(t, "hook_claude.golden.toml", tomlOut)
}

func TestHookTemplate_HasEveryEventAndIsAsync(t *testing.T) {
	out, err := HookTemplate(TemplateOptions{})
	require.NoError(t, err)
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
				Async   bool   `json:"async"`
				Timeout int    `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(out, &doc))
	for _, event := range []string{"PreToolUse", "UserPromptExpansion", "InstructionsLoaded", "SubagentStart", "SubagentStop"} {
		require.Contains(t, doc.Hooks, event)
		for _, group := range doc.Hooks[event] {
			for _, h := range group.Hooks {
				assert.True(t, h.Async, event)
				assert.Positive(t, h.Timeout, event)
			}
		}
	}
	assert.Contains(t, doc.Hooks["InstructionsLoaded"][0].Hooks[0].Command, "telemetry record")
	assert.Contains(t, doc.Hooks["PreToolUse"][0].Hooks[0].Command, "usage record")
}

func TestHookTemplate_TOMLParsesIntoHookGroups(t *testing.T) {
	out, err := HookTemplate(TemplateOptions{Format: FormatTOML, Executable: "/opt/bin/ai-rulez", Role: "it's"})
	require.NoError(t, err)
	var doc struct {
		Hooks []config.HookGroup `toml:"hooks"`
	}
	require.NoError(t, toml.Unmarshal(out, &doc))
	require.Len(t, doc.Hooks, 5)
	events := map[string]bool{}
	for _, g := range doc.Hooks {
		events[g.Event] = true
		assert.Equal(t, []string{"claude"}, g.Targets)
		require.Len(t, g.Hooks, 1)
		assert.NotEmpty(t, g.Hooks[0].Command)
	}
	assert.Len(t, events, 5)
}

func TestHookTemplate_OtherHarnessesAndErrors(t *testing.T) {
	codex, err := HookTemplate(TemplateOptions{Harness: "codex"})
	require.NoError(t, err)
	assert.NotContains(t, string(codex), "InstructionsLoaded", "Codex documents no instruction-load event")
	assert.Contains(t, string(codex), "usage record")
	assert.Contains(t, string(codex), "ai-rulez telemetry record --harness codex")
	assert.Contains(t, string(codex), `"SubagentStart"`)
	assert.Contains(t, string(codex), `"SubagentStop"`)

	cursor, err := HookTemplate(TemplateOptions{Harness: "cursor"})
	require.NoError(t, err)
	var doc struct {
		Version int `json:"version"`
		Hooks   map[string][]struct {
			Command string `json:"command"`
			Timeout int    `json:"timeout"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(cursor, &doc))
	assert.Equal(t, 1, doc.Version)
	for _, event := range []string{"subagentStart", "subagentStop"} {
		require.Len(t, doc.Hooks[event], 1, event)
		assert.Equal(t, "ai-rulez telemetry record --harness cursor", doc.Hooks[event][0].Command)
		assert.Positive(t, doc.Hooks[event][0].Timeout)
	}
	assert.Contains(t, doc.Hooks, "preToolUse", "the skill-read hook is kept")
	assert.NotContains(t, string(cursor), "InstructionsLoaded")

	_, err = HookTemplate(TemplateOptions{Harness: "codex", Format: FormatTOML})
	assert.Error(t, err)
	_, err = HookTemplate(TemplateOptions{Harness: "vim"})
	assert.Error(t, err)
	_, err = HookTemplate(TemplateOptions{Format: "yaml"})
	assert.Error(t, err)
}

func TestRecordCommand_QuotesTheExecutable(t *testing.T) {
	tests := []struct{ name, exe, want string }{
		{"plain name unchanged", "ai-rulez", "ai-rulez telemetry record"},
		{"plain path unchanged", "/opt/bin/ai-rulez", "/opt/bin/ai-rulez telemetry record"},
		{"space quoted", "/opt/my tools/ai-rulez", "'/opt/my tools/ai-rulez' telemetry record"},
		{"substitution stays literal", "/x/$(id)/ai-rulez", "'/x/$(id)/ai-rulez' telemetry record"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, recordCommand(tt.exe, "", "claude"))
		})
	}
}
