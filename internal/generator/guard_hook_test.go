package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const guardHookConfig = `version = "4.0"
name = "guard-hooks"
presets = ["claude", "codex", "gemini", "cursor", "factory", "copilot", "opencode"]

[guard]
generated = true
command = ["ai-rulez"]
`

func TestGenerate_GuardHookPerHarness(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		matcher string
	}{
		{name: "claude", file: ".claude/settings.json", matcher: `"matcher": "Edit|Write|MultiEdit"`},
		{name: "codex", file: ".codex/hooks.json", matcher: `"matcher": "apply_patch|Edit|Write"`},
		{name: "gemini", file: ".gemini/settings.json", matcher: `"matcher": "^replace$|^write_file$"`},
		{name: "cursor", file: ".cursor/hooks.json", matcher: `"matcher": "^Write$"`},
		{name: "factory", file: ".factory/hooks.json", matcher: `"matcher": "Edit|Create|ApplyPatch"`},
		{name: "copilot", file: ".github/hooks/ai-rulez.json", matcher: `"matcher": "edit|create"`},
	}
	root := writeProject(t, guardHookConfig, nil)
	generateProject(t, root)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := readProjectFile(t, root, tt.file)

			if tt.name == "copilot" {
				assert.Contains(t, got, `"bash": "ai-rulez guard"`)
			} else {
				assert.Contains(t, got, `"command": "ai-rulez guard"`)
			}
			assert.Contains(t, got, tt.matcher)
		})
	}

	t.Run("harnesses without a blocking PreToolUse hook are skipped", func(t *testing.T) {
		assert.NoFileExists(t, filepath.Join(root, ".opencode", "plugins", "ai-rulez-hooks.js"))
	})
}

func TestGenerate_GuardHookIsOptIn(t *testing.T) {
	root := writeProject(t, `version = "4.0"
name = "no-guard"
presets = ["claude"]
`, nil)

	generateProject(t, root)

	assert.NoFileExists(t, filepath.Join(root, ".claude", "settings.json"))
}

func TestGenerate_GuardHookKeepsUserHooksAndCleanRemovesIt(t *testing.T) {
	root := writeProject(t, guardHookConfig+`
[[hooks]]
event = "Stop"
targets = ["claude"]
[[hooks.hooks]]
command = "say done"
`, map[string]string{".claude/settings.json": `{"model": "opus"}`})

	gen := generateProject(t, root)
	generateProject(t, root)

	got := readProjectFile(t, root, ".claude/settings.json")
	assert.Contains(t, got, `"model": "opus"`)
	assert.Contains(t, got, "say done")
	assert.Equal(t, 1, strings.Count(got, "ai-rulez guard"), "a second run does not add the hook twice")
	_, err := gen.Clean("default", CleanOptions{})
	require.NoError(t, err)
	if data, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json")); err == nil {
		assert.NotContains(t, string(data), "ai-rulez guard")
		assert.Contains(t, string(data), `"model"`)
	}
}
