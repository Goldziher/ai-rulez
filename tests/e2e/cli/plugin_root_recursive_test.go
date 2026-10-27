package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pluginRootRepo = `version = "5.0"
name = "app"
presets = ["claude"]
gitignore = false
`

const pluginRootConfig = `version = "5.0"
name = "acme-plugin"
default_layers = ["plugins"]
`

const pluginRootLayer = `[plugin]
name = "acme"
description = "Acme skills."
version = "1.4.0"
repository = "https://github.com/acme/skills"
runtimes = ["claude"]

[plugin.author]
name = "Jane"

[[plugin.mcp]]
name = "acme-docs"
command = "acme-docs"
args = ["serve"]
`

const pluginRootSkill = "---\nname: deploy\ndescription: Use when deploying the service to production; not for local runs.\n---\n\n# Deploy\n\nRun the pipeline.\n"

func pluginRootProject(t *testing.T, env *isoEnv) string {
	t.Helper()
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".ai-rulez/config.toml":                   pluginRootRepo,
		".ai-rulez/rules/care.md":                 "# Care\n\nBe careful.\n",
		"plugin/.ai-rulez/config.toml":            pluginRootConfig,
		"plugin/.ai-rulez/config.plugins.toml":    pluginRootLayer,
		"plugin/.ai-rulez/skills/deploy/SKILL.md": pluginRootSkill,
	})
	env.git(root, "init", "-q")
	return root
}

func TestGenerateRecursive_BuildsPluginAuthoringRootAsBundle(t *testing.T) {
	// Arrange
	env := newIsoEnv(t)
	root := pluginRootProject(t, env)

	// Act
	res := env.run(root, "generate", "--recursive", "--yes")

	// Assert
	require.Equal(t, 0, res.ExitCode, res.Stdout+res.Stderr)
	verify := env.run(filepath.Join(root, "plugin"), "verify", "--plugin")
	assert.Equal(t, 0, verify.ExitCode, verify.Stdout+verify.Stderr)
	for _, rel := range []string{".mcp.json", ".gitignore", ".ai-rulez/.generated-manifest.local.json"} {
		_, err := os.Stat(filepath.Join(root, "plugin", rel))
		assert.True(t, os.IsNotExist(err), "%s must not be written into the plugin root", rel)
	}
	assert.FileExists(t, filepath.Join(root, "CLAUDE.md"), "the project root still generates normally")
}

func TestGenerateCheckRecursive_SkipsPluginAuthoringRoot(t *testing.T) {
	// Arrange
	env := newIsoEnv(t)
	root := pluginRootProject(t, env)
	require.Equal(t, 0, env.run(root, "generate", "--recursive", "--yes").ExitCode)

	// Act
	res := env.run(root, "generate", "--check", "--recursive")

	// Assert
	assert.Equal(t, 0, res.ExitCode, res.Stdout+res.Stderr)
	assert.Contains(t, res.Stdout+res.Stderr, "verify --plugin")
}
