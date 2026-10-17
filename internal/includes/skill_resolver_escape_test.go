package includes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestResolveInstalledSkills_PathCannotEscapeTheLocalSource(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"dotdot out of the source", "../outside/skills/x"},
		{"dotdot after a real directory", "skills/../../outside/skills/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			project := filepath.Join(root, "project")
			writeSkill(t, filepath.Join(project, "skills", "inside"), "inside")
			writeSkill(t, filepath.Join(root, "outside", "skills", "x"), "x")
			cfg := &config.Config{BaseDir: project, InstalledSkills: []config.InstalledSkillConfig{{Name: "x", Source: ".", Path: tt.path}}}
			// Act
			skills, err := ResolveInstalledSkills(t.Context(), cfg, "")
			// Assert
			require.Error(t, err, "the committed config must not read outside the project, got %v", resolvedNames(skills))
			assert.Empty(t, skills)
		})
	}
}

func TestResolveInstalledSkills_SymlinkedPathComponentCannotLeaveTheProject(t *testing.T) {
	// Arrange: skills/link inside the project is a symlink to a skill directory outside it
	root := t.TempDir()
	project := filepath.Join(root, "project")
	require.NoError(t, os.MkdirAll(filepath.Join(project, "skills"), 0o755))
	writeSkill(t, filepath.Join(root, "outside", "secret"), "secret")
	testutil.SymlinkOrSkip(t, filepath.Join(root, "outside", "secret"), filepath.Join(project, "skills", "link"))
	cfg := &config.Config{BaseDir: project, InstalledSkills: []config.InstalledSkillConfig{{Name: "link", Source: "."}}}
	// Act
	skills, err := ResolveInstalledSkills(t.Context(), cfg, "")
	// Assert
	require.Error(t, err, "the symlink leaves the project, got %v", resolvedNames(skills))
	assert.Empty(t, skills)
}
