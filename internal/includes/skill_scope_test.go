package includes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestResolveInstalledSkills_LocalSourceStaysInsideProject(t *testing.T) {
	// Arrange: <root>/project (the project), <root>/victim (outside, holds a
	// skill), and a link inside the project that points at the victim.
	root := t.TempDir()
	project := filepath.Join(root, "project")
	victim := filepath.Join(root, "victim")
	require.NoError(t, os.MkdirAll(filepath.Join(project, "vendor", "demo"), 0o755))
	require.NoError(t, os.MkdirAll(victim, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project, "vendor", "demo", "SKILL.md"), []byte("# Demo\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(victim, "SKILL.md"), []byte("# Victim\n"), 0o644))
	testutil.SymlinkOrSkip(t, victim, filepath.Join(project, "link"))

	committedOverlay := func(tracked bool) *config.LocalOverlay {
		return &config.LocalOverlay{
			Tracked: tracked,
			Doc:     map[string]any{"installed_skills": []any{map[string]any{"name": "demo", "source": "../victim"}}},
		}
	}

	tests := []struct {
		name    string
		source  string
		cfg     config.Config
		wantErr bool
	}{
		{"relative inside", "vendor/demo", config.Config{}, false},
		{"absolute inside", filepath.Join(project, "vendor", "demo"), config.Config{}, false},
		{"dot-dot outside", "../victim", config.Config{}, true},
		{"absolute outside", victim, config.Config{}, true},
		{"symlink inside pointing outside", "link", config.Config{}, true},
		{"outside allowed from the local overlay", "../victim", config.Config{LocalOverlay: committedOverlay(false)}, false},
		{"outside refused from a git-tracked overlay", "../victim", config.Config{LocalOverlay: committedOverlay(true)}, true},
		{"outside allowed in user scope", "../victim", config.Config{UserScope: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := tt.cfg
			cfg.BaseDir = project
			cfg.InstalledSkills = []config.InstalledSkillConfig{{Name: "demo", Source: tt.source, Path: "."}}

			// Act
			skills, err := ResolveInstalledSkills(t.Context(), &cfg, "")

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, config.ErrIncludeOutsideProject)
				assert.ErrorIs(t, err, config.ErrSkillUnresolved)
				assert.Contains(t, err.Error(), "outside the project")
				return
			}
			require.NoError(t, err)
			require.Len(t, skills, 1)
			assert.Equal(t, "demo", skills[0].Name)
		})
	}
}

func TestResolveInstalledSkills_FileURLStaysInsideProject(t *testing.T) {
	// Arrange: a file:// source outside the project is refused before any git
	// command runs.
	t.Setenv(lockfile.EnvAllowFileURLs, "")
	root := t.TempDir()
	project := filepath.Join(root, "project")
	victim := filepath.Join(root, "victim")
	require.NoError(t, os.MkdirAll(project, 0o755))
	require.NoError(t, os.MkdirAll(victim, 0o755))
	cfg := &config.Config{
		BaseDir:         project,
		InstalledSkills: []config.InstalledSkillConfig{{Name: "demo", Source: "file://" + filepath.ToSlash(victim)}},
	}

	// Act
	_, err := ResolveInstalledSkills(t.Context(), cfg, "")

	// Assert
	require.ErrorIs(t, err, config.ErrIncludeOutsideProject)
}
