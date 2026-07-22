package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupGeneratedProject copies the basic fixture into a temp dir, generates with
// gitignore enabled, and returns the project dir + generator ready for cleaning.
func setupGeneratedProject(t *testing.T) (string, *Generator) {
	t.Helper()
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "basic")
	tempDir := t.TempDir()
	copyFixture(t, fixtureDir, tempDir)

	cfg, err := config.LoadConfig(context.Background(), tempDir)
	require.NoError(t, err)
	enabled := true
	cfg.Gitignore = &enabled

	gen := NewGenerator(cfg)
	require.NoError(t, gen.Generate("default"))

	// Sanity: generate produced outputs, a manifest, and a gitignore block.
	require.DirExists(t, filepath.Join(tempDir, ".claude"))
	require.FileExists(t, filepath.Join(tempDir, ".ai-rulez", generatedManifestName))
	require.FileExists(t, filepath.Join(tempDir, ".gitignore"))

	return tempDir, gen
}

func TestGenerator_Clean_RemovesGeneratedOutputs(t *testing.T) {
	tempDir, gen := setupGeneratedProject(t)

	plan, err := gen.Clean("default", CleanOptions{})
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.NotEmpty(t, plan.Files, "expected generated files in the plan")

	// Generated outputs are gone.
	assert.NoDirExists(t, filepath.Join(tempDir, ".claude"))
	assert.NoFileExists(t, filepath.Join(tempDir, "CLAUDE.md"))

	// Manifest is gone.
	assert.NoFileExists(t, filepath.Join(tempDir, ".ai-rulez", generatedManifestName))

	// The ai-rulez managed .gitignore block is stripped (file removed when it
	// held only the managed block).
	if data, err := os.ReadFile(filepath.Join(tempDir, ".gitignore")); err == nil {
		assert.NotContains(t, string(data), "CLAUDE.md")
	}

	// The .ai-rulez source tree is untouched.
	assert.FileExists(t, filepath.Join(tempDir, ".ai-rulez", "config.yaml"))
	assert.FileExists(t, filepath.Join(tempDir, ".ai-rulez", "rules", "coding-style.md"))
	assert.DirExists(t, filepath.Join(tempDir, ".ai-rulez"))
}

func TestGenerator_Clean_DryRunDeletesNothing(t *testing.T) {
	tempDir, gen := setupGeneratedProject(t)

	plan, err := gen.Clean("default", CleanOptions{DryRun: true})
	require.NoError(t, err)
	assert.NotEmpty(t, plan.Files)

	// Nothing was actually removed.
	assert.DirExists(t, filepath.Join(tempDir, ".claude"))
	assert.FileExists(t, filepath.Join(tempDir, ".ai-rulez", generatedManifestName))
	assert.FileExists(t, filepath.Join(tempDir, ".gitignore"))
}

func TestGenerator_Clean_KeepFlags(t *testing.T) {
	tempDir, gen := setupGeneratedProject(t)

	plan, err := gen.Clean("default", CleanOptions{KeepGitignore: true, KeepManifest: true})
	require.NoError(t, err)
	assert.Empty(t, plan.ManifestPath, "manifest should not be scheduled for removal when kept")
	assert.False(t, plan.GitignoreEdited, "gitignore should not be edited when kept")

	// Generated outputs still removed, but manifest + gitignore preserved.
	assert.NoDirExists(t, filepath.Join(tempDir, ".claude"))
	assert.FileExists(t, filepath.Join(tempDir, ".ai-rulez", generatedManifestName))
	data, err := os.ReadFile(filepath.Join(tempDir, ".gitignore"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "CLAUDE.md")
}

func TestGenerator_Clean_PreservesUserFilesInGeneratedDir(t *testing.T) {
	tempDir, gen := setupGeneratedProject(t)

	// A user-authored file living inside a generated directory must survive: the
	// directory is non-empty after clean, so it is not removed.
	userFile := filepath.Join(tempDir, ".claude", "user-notes.md")
	require.NoError(t, os.WriteFile(userFile, []byte("keep me"), 0o644))

	_, err := gen.Clean("default", CleanOptions{})
	require.NoError(t, err)

	assert.FileExists(t, userFile)
	assert.DirExists(t, filepath.Join(tempDir, ".claude"))
}

func TestGenerator_Clean_Idempotent(t *testing.T) {
	_, gen := setupGeneratedProject(t)

	// First clean removes everything.
	first, err := gen.Clean("default", CleanOptions{})
	require.NoError(t, err)
	assert.False(t, first.Empty())

	// Second clean has nothing left to do.
	second, err := gen.Clean("default", CleanOptions{})
	require.NoError(t, err)
	assert.True(t, second.Empty(), "a second clean should find nothing to remove")
}
