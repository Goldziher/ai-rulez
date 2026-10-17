package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const rootLayoutInstalled = rootLayoutConfig + "\n[[installed_skills]]\nname = \"demo\"\nsource = \"./shared\"\n"

func TestRootLayoutCommands_RunWithoutAContentTree(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{"validate", []string{"validate"}, 0},
		{"validate strict", []string{"validate", "--strict"}, 0},
		{"doctor", []string{"doctor"}, 0},
		{"generate dry run", []string{"generate", "--dry-run", "--yes"}, 0},
		{"lock", []string{"lock"}, 0},
		{"list skills", []string{"list", "skills"}, 0},
		{"list rules", []string{"list", "rules"}, 0},
		{"domain list", []string{"domain", "list"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := rootLayoutProject(t, rootLayoutInstalled)

			// Act
			res := runCLI(t, tt.args...)

			// Assert
			assert.Equal(t, tt.wantCode, res.code, res.stdout+res.stderr)
			assert.NoDirExists(t, filepath.Join(root, ".ai-rulez"))
		})
	}
}

func TestRootLayoutCommands_ContentWritesAreRefused(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"add rule", []string{"add", "rule", "style"}},
		{"add context", []string{"add", "context", "notes"}},
		{"add skill", []string{"add", "skill", "deploy", "--description", "d"}},
		{"add agent", []string{"add", "agent", "reviewer"}},
		{"add command", []string{"add", "command", "ship"}},
		{"add check", []string{"add", "check", "sec"}},
		{"domain add", []string{"domain", "add", "backend"}},
		{"remove rule", []string{"remove", "rule", "style", "--yes"}},
		{"domain remove", []string{"domain", "remove", "backend", "--yes"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := rootLayoutProject(t, rootLayoutConfig)

			// Act
			res := runCLI(t, tt.args...)

			// Assert
			assert.Equal(t, 1, res.code, res.stdout+res.stderr)
			assert.Contains(t, res.stdout+res.stderr, "no .ai-rulez/ directory")
			assert.NoDirExists(t, filepath.Join(root, ".ai-rulez"))
			assert.Equal(t, rootLayoutConfig, readFileString(t, filepath.Join(root, "ai-rulez.toml")))
		})
	}
}

func TestRootLayoutValidate_RejectsABrokenFile(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"invalid toml", "version = \"5.0\"\nname = \n", "parse TOML config"},
		{"default layers", rootLayoutConfig + "default_layers = [\"ci\"]\n", "default_layers is not supported in ai-rulez.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			rootLayoutProject(t, tt.body)

			// Act
			res := runCLI(t, "validate")

			// Assert
			assert.Equal(t, 1, res.code, res.stdout+res.stderr)
			assert.Contains(t, res.stdout+res.stderr, tt.want)
		})
	}
}

func TestRootLayoutLock_PinsAndChecksTheInstalledSkill(t *testing.T) {
	// Arrange
	root := rootLayoutProject(t, rootLayoutInstalled)

	// Act
	locked := runCLI(t, "lock")
	checked := runCLI(t, "lock", "--check")
	writeFile(t, filepath.Join(root, "shared", "skills", "demo", "SKILL.md"), rootLayoutSkill+"changed\n")
	drifted := runCLI(t, "lock", "--check")

	// Assert
	require.Equal(t, 0, locked.code, locked.stdout+locked.stderr)
	assert.Contains(t, readFileString(t, filepath.Join(root, "ai-rulez.lock")), "demo")
	assert.Equal(t, 0, checked.code, checked.stdout+checked.stderr)
	assert.Equal(t, 2, drifted.code, drifted.stdout+drifted.stderr)
}
