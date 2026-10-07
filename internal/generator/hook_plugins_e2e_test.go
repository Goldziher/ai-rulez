package generator

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hookPluginsProjectConfig = `version = "5.0"
name = "hook-plugins"
presets = ["opencode", "kilo", "mimocode", "pi", "amp"]
gitignore = true

[[hooks]]
event = "PreToolUse"
matcher = "Bash"
[[hooks.hooks]]
command = "echo guard"

[[hooks]]
event = "Stop"
targets = ["pi"]
[[hooks.hooks]]
command = "echo done"
`

var hookPluginFiles = []string{
	".opencode/plugins/ai-rulez-hooks.js",
	".kilo/plugins/ai-rulez-hooks.js",
	".mimocode/plugins/ai-rulez-hooks.js",
	".pi/extensions/ai-rulez-hooks.ts",
	".amp/plugins/ai-rulez-hooks.ts",
}

func TestGenerate_HookPluginsAreGitignoredAndCleaned(t *testing.T) {
	root := writeProject(t, hookPluginsProjectConfig, nil)

	gen := generateProject(t, root)

	for _, rel := range hookPluginFiles {
		assert.FileExists(t, filepath.Join(root, filepath.FromSlash(rel)))
		ignore := readProjectFile(t, root, ".gitignore")
		assert.Contains(t, ignore, "\n"+rel+"\n", "the module itself is ignored")
		assert.NotContains(t, ignore, "\n"+path.Dir(rel)+"/\n", "the plugin directory also holds hand-written plugins")
	}
	assert.Contains(t, readProjectFile(t, root, ".pi/extensions/ai-rulez-hooks.ts"), "echo done")
	assert.NotContains(t, readProjectFile(t, root, ".amp/plugins/ai-rulez-hooks.ts"), "echo done",
		"a group that targets pi is not rendered for amp")

	// A second run leaves the modules as they are.
	before := readProjectFile(t, root, ".opencode/plugins/ai-rulez-hooks.js")
	generateProject(t, root)
	assert.Equal(t, before, readProjectFile(t, root, ".opencode/plugins/ai-rulez-hooks.js"))

	// clean removes what ai-rulez wrote.
	_, err := gen.Clean("default", CleanOptions{})
	require.NoError(t, err)
	for _, rel := range hookPluginFiles {
		assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(rel)))
	}
}

func TestGenerate_NoHooksWritesNoPlugins(t *testing.T) {
	root := writeProject(t, `version = "5.0"
name = "plain"
presets = ["opencode", "kilo", "mimocode", "pi", "amp"]
gitignore = false
`, nil)

	generateProject(t, root)

	for _, rel := range hookPluginFiles {
		assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(rel)))
	}
}

func TestGenerate_DroppingHooksRemovesThePlugins(t *testing.T) {
	root := writeProject(t, hookPluginsProjectConfig, nil)
	generateProject(t, root)
	require.FileExists(t, filepath.Join(root, ".pi", "extensions", "ai-rulez-hooks.ts"))

	trimmed := `version = "5.0"
name = "hook-plugins"
presets = ["opencode", "kilo", "mimocode", "pi", "amp"]
gitignore = true
`
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "config.toml"), []byte(trimmed), 0o644))
	generateProject(t, root)

	for _, rel := range hookPluginFiles {
		assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(rel)), "stale module of a dropped hook")
	}
}
