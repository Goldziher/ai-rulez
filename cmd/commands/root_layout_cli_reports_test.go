package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootLayoutReports_ReadTheConfigWithoutAContentTree(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"catalog", []string{"catalog"}, ""},
		{"catalog json", []string{"catalog", "--format", "json"}, "{"},
		{"sbom", []string{"sbom"}, ""},
		{"tokens", []string{"tokens"}, ""},
		{"cost", []string{"cost"}, ""},
		{"roles", []string{"roles"}, ""},
		{"scan", []string{"scan"}, ""},
		{"verifiers list", []string{"verifiers", "list"}, ""},
		{"validate json", []string{"validate", "--format", "json"}, "{"},
		{"doctor json", []string{"doctor", "--format", "json"}, "{"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := rootLayoutProject(t, rootLayoutInstalled)

			// Act
			res := runCLI(t, tt.args...)

			// Assert
			assert.Equal(t, 0, res.code, res.stdout+res.stderr)
			assert.Contains(t, res.stdout, tt.want)
			assert.NoDirExists(t, filepath.Join(root, ".ai-rulez"))
		})
	}
}

func TestRootLayoutClean_RemovesOnlyWhatGenerateWrote(t *testing.T) {
	// Arrange
	root := rootLayoutProject(t, rootLayoutInstalled)
	writeFile(t, filepath.Join(root, "CLAUDE.md"), "# Hand written\n")
	writeFile(t, filepath.Join(root, "AGENTS.md"), "# Hand written\n")
	require.Equal(t, 0, runCLI(t, "generate", "--yes").code)
	require.FileExists(t, filepath.Join(root, ".claude", "skills", "demo", "SKILL.md"))
	require.FileExists(t, filepath.Join(root, ".ai-rulez-manifest.json"))

	// Act
	dry := runCLI(t, "clean", "--dry-run")
	stillThere := fileExists(filepath.Join(root, ".claude", "skills", "demo", "SKILL.md"))
	clean := runCLI(t, "clean", "--yes")

	// Assert
	assert.Equal(t, 0, dry.code, dry.stdout+dry.stderr)
	assert.True(t, stillThere, "a dry run removes nothing")
	require.Equal(t, 0, clean.code, clean.stdout+clean.stderr)
	assert.NoFileExists(t, filepath.Join(root, ".claude", "skills", "demo", "SKILL.md"))
	assert.NoFileExists(t, filepath.Join(root, ".ai-rulez-manifest.json"))
	assert.NoFileExists(t, filepath.Join(root, ".ai-rulez-manifest.local.json"))
	assert.Equal(t, "# Hand written\n", readFileString(t, filepath.Join(root, "CLAUDE.md")))
	assert.Equal(t, "# Hand written\n", readFileString(t, filepath.Join(root, "AGENTS.md")))
	assert.FileExists(t, filepath.Join(root, "ai-rulez.toml"))
}

func TestRootLayoutGenerate_CheckDetectsDriftOfAnInstalledSkill(t *testing.T) {
	// Arrange
	root := rootLayoutProject(t, rootLayoutInstalled)
	require.Equal(t, 0, runCLI(t, "generate", "--yes").code)
	clean := runCLI(t, "generate", "--check")
	require.NoError(t, os.Remove(filepath.Join(root, ".claude", "skills", "demo", "SKILL.md")))

	// Act
	drifted := runCLI(t, "generate", "--check")

	// Assert
	assert.Equal(t, 0, clean.code, clean.stdout+clean.stderr)
	assert.Equal(t, 2, drifted.code, drifted.stdout+drifted.stderr)
}
