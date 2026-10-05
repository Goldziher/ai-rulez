package generator

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sharedVSCodeConfig = `version = "4.0"
name = "shared"
presets = ["copilot", "zoocode"]
gitignore = false

[permissions]
allow = ["Bash(npm run test:*)"]
deny = ["Bash(rm -rf:*)"]
`

const sharedVSCodeHandAuthored = "{\n  // my editor\n  \"editor.tabSize\": 2\n}\n"

func TestGenerate_PresetsMergingDifferentKeysIntoOneDocumentAreUnioned(t *testing.T) {
	quietWarnings(t)
	root := writeProject(t, sharedVSCodeConfig, map[string]string{".vscode/settings.json": sharedVSCodeHandAuthored})

	gen := generateProject(t, root)

	body := readProjectFile(t, root, ".vscode/settings.json")
	assert.Contains(t, body, "chat.tools.terminal.autoApprove", "copilot's keys")
	assert.Contains(t, body, "zoo-code.deniedCommands", "zoocode's keys")
	assert.Contains(t, body, "// my editor")

	// A second run changes nothing, and clean returns the hand-authored file.
	generateProject(t, root)
	assert.Equal(t, body, readProjectFile(t, root, ".vscode/settings.json"))
	_, err := gen.Clean("default", CleanOptions{})
	require.NoError(t, err)
	assert.Equal(t, sharedVSCodeHandAuthored, readProjectFile(t, root, ".vscode/settings.json"))
}

const sharedHooksConfig = `version = "4.0"
name = "shared"
presets = ["copilot", "copilot-cli"]
gitignore = false

[[hooks]]
event = "PreToolUse"
targets = ["copilot"]
[[hooks.hooks]]
command = "echo editor"

[[hooks]]
event = "PreToolUse"
targets = ["copilot-cli"]
[[hooks.hooks]]
command = "echo cli"
`

func TestGenerate_PresetsOwningOneHooksFileWithDifferentTargetsAreUnioned(t *testing.T) {
	quietWarnings(t)
	root := writeProject(t, sharedHooksConfig, nil)

	gen := generateProject(t, root)

	body := readProjectFile(t, root, ".github/hooks/ai-rulez.json")
	assert.Contains(t, body, "echo editor")
	assert.Contains(t, body, "echo cli")

	_, err := gen.Clean("default", CleanOptions{})
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(root, ".github", "hooks", "ai-rulez.json"))
}
