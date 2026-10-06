package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFinalFilesAreWhatTheDiskApplierWrites(t *testing.T) {
	// Arrange
	dir := planProject(t, applyConfig)
	g := newApplyGenerator(t, dir)
	plan, err := g.Plan("")
	require.NoError(t, err)

	// Act
	final := plan.FinalFiles()
	_, err = g.Apply(plan, DiskApplier)
	require.NoError(t, err)

	// Assert: every planned file holds, byte for byte, what the run wrote.
	require.NotEmpty(t, final)
	for rel, want := range final {
		got, readErr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		require.NoError(t, readErr, rel)
		assert.Equal(t, string(want), string(got), rel)
	}
}

func TestPlannedFilesAreQuietAndLeaveTheConfigAlone(t *testing.T) {
	// Arrange
	dir := planProject(t, applyConfig)
	cfg := loadPlanConfig(t, dir, nil)
	hooks := len(cfg.Hooks)

	// Act
	planned := NewPlannedFiles(cfg)
	paths := planned.PlannedPaths()
	content, ok := planned.Planned("CLAUDE.md")

	// Assert
	assert.Contains(t, paths, "CLAUDE.md")
	require.True(t, ok)
	assert.Contains(t, string(content), "GENERATED FILE")
	assert.Nil(t, cfg.Diag, "the render works on a copy")
	assert.Len(t, cfg.Hooks, hooks)
	_, statErr := os.Stat(filepath.Join(dir, "CLAUDE.md"))
	assert.True(t, os.IsNotExist(statErr), "planning writes nothing")
}
