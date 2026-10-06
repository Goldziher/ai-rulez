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
	assert.NotContains(t, string(codex), "InstructionsLoaded", "no verified equivalent for codex")
	assert.Contains(t, string(codex), "usage record")

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
			assert.Equal(t, tt.want, recordCommand(tt.exe, ""))
		})
	}
}
