package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const userOwnedFilesConfig = `version = "5.0"
name = "me"
presets = ["claude", "codex", "cline", "opencode", "copilot", "pi"]

[[hooks]]
event = "PreToolUse"
matcher = "Bash"
[[hooks.hooks]]
command = "echo guard"

[permissions]
deny = ["Bash(rm -rf:*)"]
`

const userOwnedFilesBare = `version = "5.0"
name = "me"
presets = ["claude", "codex", "cline", "opencode", "copilot", "pi"]
`

// userOwnedFiles are whole files ai-rulez owns whose extension cannot carry a header.
var userOwnedFiles = []string{
	".codex/rules/ai-rulez.rules",
	".copilot/hooks/ai-rulez.json",
	".config/opencode/plugins/ai-rulez-hooks.js",
	".pi/agent/extensions/ai-rulez-hooks.ts",
}

func TestUser_OwnedWholeFilesAreRemovedWhenTheirSourceGoes(t *testing.T) {
	quietWarnings(t)
	home, gen := newUserHome(t, userOwnedFilesConfig, nil)
	_, err := gen.GenerateUser("")
	require.NoError(t, err)
	for _, rel := range userOwnedFiles {
		require.FileExists(t, filepath.Join(home, filepath.FromSlash(rel)), rel)
	}

	require.NoError(t, os.WriteFile(filepath.Join(home, ".config", "ai-rulez", "config.toml"), []byte(userOwnedFilesBare), 0o644))
	_, err = loadUserGenerator(t, home).GenerateUser("")
	require.NoError(t, err)

	for _, rel := range userOwnedFiles {
		assert.NoFileExists(t, filepath.Join(home, filepath.FromSlash(rel)), rel)
	}
}

func TestUser_CleanRemovesOwnedWholeFiles(t *testing.T) {
	quietWarnings(t)
	home, gen := newUserHome(t, userOwnedFilesConfig, nil)
	_, err := gen.GenerateUser("")
	require.NoError(t, err)

	_, err = loadUserGenerator(t, home).Clean("", CleanOptions{KeepGitignore: true})
	require.NoError(t, err)

	for _, rel := range userOwnedFiles {
		assert.NoFileExists(t, filepath.Join(home, filepath.FromSlash(rel)), rel)
	}
}

func TestUser_StaleKeepsAnEditedOwnedFile(t *testing.T) {
	quietWarnings(t)
	home, gen := newUserHome(t, userOwnedFilesConfig, nil)
	_, err := gen.GenerateUser("")
	require.NoError(t, err)
	mine := filepath.Join(home, ".codex", "rules", "ai-rulez.rules")
	require.NoError(t, os.WriteFile(mine, []byte("prefix_rule(pattern = [\"ls\"], decision = \"allow\")\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".config", "ai-rulez", "config.toml"), []byte(userOwnedFilesBare), 0o644))

	_, err = loadUserGenerator(t, home).GenerateUser("")
	require.NoError(t, err)

	assert.FileExists(t, mine, "a rules file the user rewrote no longer looks generated")
}
