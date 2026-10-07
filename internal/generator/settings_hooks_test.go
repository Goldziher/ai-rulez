package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const hooksProjectConfig = `version = "5.0"
name = "hooks"
presets = ["claude", "codex", "cursor", "gemini", "copilot"]
gitignore = false

[[hooks]]
event = "Stop"
[[hooks.hooks]]
command = "echo done"

[permissions]
allow = ["Bash(git status)"]

[claude.settings.managed]
skill_overrides = { init = "off" }
`

const handAuthoredClaudeSettings = `{
  "model": "opus",
  "permissions": {"allow": ["Bash(make test)"]},
  "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "say stop"}]}]}
}
`

const handAuthoredCodexHooks = `{
  "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "say stop"}]}]}
}
`

func readProjectFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	require.NoError(t, err)
	return string(data)
}

func generateProject(t *testing.T, root string) *Generator {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	gen := NewGenerator(cfg)
	require.NoError(t, gen.Generate("default"))
	return gen
}

func TestGenerate_HooksAndPermissionsKeepHandAuthoredKeys(t *testing.T) {
	root := writeProject(t, hooksProjectConfig, map[string]string{
		".claude/settings.json": handAuthoredClaudeSettings,
		".codex/hooks.json":     handAuthoredCodexHooks,
	})

	gen := generateProject(t, root)

	claude := readProjectFile(t, root, ".claude/settings.json")
	for _, want := range []string{`"model": "opus"`, "Bash(make test)", "say stop", "echo done", "Bash(git status)", `"init": "off"`} {
		assert.Contains(t, claude, want)
	}
	codex := readProjectFile(t, root, ".codex/hooks.json")
	assert.Contains(t, codex, "say stop")
	assert.Contains(t, codex, "echo done")
	assert.FileExists(t, filepath.Join(root, ".cursor", "hooks.json"))
	assert.Contains(t, readProjectFile(t, root, ".gemini/settings.json"), "AfterAgent")
	assert.Contains(t, readProjectFile(t, root, ".github/hooks/ai-rulez.json"), "agentStop")

	// A second run is a no-op on the merged documents.
	before := claude
	generateProject(t, root)
	assert.Equal(t, before, readProjectFile(t, root, ".claude/settings.json"))

	// clean removes what ai-rulez wrote and nothing else.
	_, err := gen.Clean("default", CleanOptions{})
	require.NoError(t, err)
	claude = readProjectFile(t, root, ".claude/settings.json")
	assert.Contains(t, claude, `"model": "opus"`)
	assert.Contains(t, claude, "Bash(make test)")
	assert.Contains(t, claude, "say stop")
	for _, gone := range []string{"echo done", "Bash(git status)", "skillOverrides"} {
		assert.NotContains(t, claude, gone)
	}
	codex = readProjectFile(t, root, ".codex/hooks.json")
	assert.Contains(t, codex, "say stop")
	assert.NotContains(t, codex, "echo done")
	assert.NoFileExists(t, filepath.Join(root, ".cursor", "hooks.json"), "a file ai-rulez wrote whole goes on clean")
	assert.NoFileExists(t, filepath.Join(root, ".github", "hooks", "ai-rulez.json"))
}

func TestGenerate_DroppingAHookTakesItOutOfTheHandAuthoredFile(t *testing.T) {
	root := writeProject(t, hooksProjectConfig, map[string]string{".claude/settings.json": handAuthoredClaudeSettings})
	generateProject(t, root)
	require.Contains(t, readProjectFile(t, root, ".claude/settings.json"), "echo done")

	// Removing [[hooks]] from the config removes exactly that hook.
	configPath := filepath.Join(root, ".ai-rulez", "config.toml")
	trimmed := `version = "5.0"
name = "hooks"
presets = ["claude"]
gitignore = false
`
	require.NoError(t, os.WriteFile(configPath, []byte(trimmed), 0o644))
	generateProject(t, root)

	claude := readProjectFile(t, root, ".claude/settings.json")
	assert.NotContains(t, claude, "echo done")
	assert.NotContains(t, claude, "Bash(git status)")
	assert.Contains(t, claude, "say stop")
	assert.Contains(t, claude, "Bash(make test)")
}

func TestGenerate_NoSettingsBlocksLeavesSettingsUntouched(t *testing.T) {
	root := writeProject(t, `version = "5.0"
name = "plain"
presets = ["claude", "codex", "cursor", "gemini", "copilot"]
gitignore = false
`, nil)
	generateProject(t, root)
	assert.NoFileExists(t, filepath.Join(root, ".claude", "settings.json"))
	assert.NoFileExists(t, filepath.Join(root, ".codex", "hooks.json"))
	assert.NoFileExists(t, filepath.Join(root, ".cursor", "hooks.json"))
	assert.NoFileExists(t, filepath.Join(root, ".github", "hooks", "ai-rulez.json"))
}
