package commands

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const rootLayoutGitSource = "https://github.com/example/skills"

func TestRootLayoutSkillInstall_FlagsLandInAiRulezToml(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		target string
		want   []string
	}{
		{
			"every flag",
			[]string{"--source", "./shared", "--include", "de*", "--exclude", "x*", "--resources", "docs/*.md", "--name-prefix", "p-", "--trust", "warn"},
			"ai-rulez.toml",
			[]string{`name = "demo"`, `source = "./shared"`, `include = ["de*"]`, `exclude = ["x*"]`, `resources = ["docs/*.md"]`, `name_prefix = "p-"`, `trust = "warn"`},
		},
		{
			"version range of a git source",
			[]string{"--source", rootLayoutGitSource, "--version", "^1.2"},
			"ai-rulez.toml",
			[]string{`source = "` + rootLayoutGitSource + `"`, `version = "^1.2"`},
		},
		{
			"git ref and path",
			[]string{"--source", rootLayoutGitSource, "--ref", "v1.0.0", "--path", "skills/demo"},
			"ai-rulez.toml",
			[]string{`ref = "v1.0.0"`, `path = "skills/demo"`},
		},
		{"machine-local", []string{"--source", "./shared", "--local"}, "ai-rulez.local.toml", []string{`name = "demo"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := rootLayoutProject(t, rootLayoutConfig)

			// Act
			res := runCLI(t, append([]string{"skill", "install", "demo"}, tt.args...)...)

			// Assert
			require.Equal(t, 0, res.code, res.stdout+res.stderr)
			body := strings.ReplaceAll(readFileString(t, filepath.Join(root, tt.target)), "'", `"`) // the writer quotes with single quotes
			for _, want := range tt.want {
				assert.Contains(t, body, want)
			}
			assert.NoFileExists(t, filepath.Join(root, "config.toml"))
			assert.NoDirExists(t, filepath.Join(root, ".ai-rulez"))
		})
	}
}

func TestRootLayoutSkillInstall_RefusesBadInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no source", []string{"skill", "install", "demo"}, "source"},
		{"bad trust level", []string{"skill", "install", "demo", "--source", "./shared", "--trust", "maybe"}, "trust"},
		{"version and ref together", []string{"skill", "install", "demo", "--source", rootLayoutGitSource, "--version", "^1.0", "--ref", "main"}, "ref"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := rootLayoutProject(t, rootLayoutConfig)

			// Act
			res := runCLI(t, tt.args...)

			// Assert
			assert.NotEqual(t, 0, res.code, res.stdout+res.stderr)
			assert.Contains(t, res.stdout+res.stderr, tt.want)
			assert.Equal(t, rootLayoutConfig, readFileString(t, filepath.Join(root, "ai-rulez.toml")))
		})
	}
}

func TestRootLayoutSkillInstall_RejectsADuplicateAndRemoveUndoesIt(t *testing.T) {
	// Arrange
	root := rootLayoutProject(t, rootLayoutConfig)
	require.Equal(t, 0, runCLI(t, "skill", "install", "demo", "--source", "./shared").code)

	// Act
	again := runCLI(t, "skill", "install", "demo", "--source", "./shared")
	list := runCLI(t, "skill", "list")
	removed := runCLI(t, "skill", "remove", "demo", "--yes")
	missing := runCLI(t, "skill", "remove", "demo", "--yes")

	// Assert
	assert.Equal(t, 1, again.code)
	assert.Contains(t, again.stdout+again.stderr, "already installed")
	assert.Contains(t, list.stdout, "demo")
	assert.Equal(t, 0, removed.code, removed.stdout+removed.stderr)
	assert.NotContains(t, readFileString(t, filepath.Join(root, "ai-rulez.toml")), "installed_skills")
	assert.Equal(t, 1, missing.code)
	assert.Contains(t, missing.stdout+missing.stderr, "not found")
}

func TestRootLayoutInclude_AddAndRemoveEditAiRulezToml(t *testing.T) {
	// Arrange
	root := rootLayoutProject(t, rootLayoutConfig)
	writeFile(t, filepath.Join(root, "shared", ".ai-rulez", "rules", "r.md"), "# R\n")

	// Act
	added := runCLI(t, "include", "add", "team", "./shared", "--include", "rules")
	list := runCLI(t, "include", "list")
	removed := runCLI(t, "include", "remove", "team", "--yes")

	// Assert
	require.Equal(t, 0, added.code, added.stdout+added.stderr)
	assert.Contains(t, list.stdout, "team")
	require.Equal(t, 0, removed.code, removed.stdout+removed.stderr)
	assert.NotContains(t, readFileString(t, filepath.Join(root, "ai-rulez.toml")), "includes")
	assert.NoFileExists(t, filepath.Join(root, "config.toml"))
	assert.NoDirExists(t, filepath.Join(root, ".ai-rulez"))
}

func TestRootLayoutLocal_OverlayCommandsUseTheRootOverlay(t *testing.T) {
	// Arrange
	root := rootLayoutProject(t, rootLayoutConfig)
	overlay := filepath.Join(root, "ai-rulez.local.toml")

	// Act
	empty := runCLI(t, "local", "show")
	initRun := runCLI(t, "local", "init")
	set := runCLI(t, "local", "set", "description", "mine")
	show := runCLI(t, "local", "show")
	path := runCLI(t, "local", "path")
	unset := runCLI(t, "local", "unset", "description")

	// Assert
	assert.Contains(t, empty.stdout, "No local overlay")
	require.Equal(t, 0, initRun.code, initRun.stdout+initRun.stderr)
	assert.FileExists(t, overlay)
	require.Equal(t, 0, set.code, set.stdout+set.stderr)
	assert.Contains(t, show.stdout, "description")
	assert.Contains(t, path.stdout, "ai-rulez.local.toml")
	require.Equal(t, 0, unset.code, unset.stdout+unset.stderr)
	assert.NotContains(t, readFileString(t, overlay), "mine")
	assert.Equal(t, rootLayoutConfig, readFileString(t, filepath.Join(root, "ai-rulez.toml")))
	assert.NoDirExists(t, filepath.Join(root, ".ai-rulez"))
}
