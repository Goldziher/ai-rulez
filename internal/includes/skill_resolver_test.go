package includes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveInstalledSkills_LocalSource(t *testing.T) {
	t.Parallel()

	t.Run("resolves local skill", func(t *testing.T) {
		t.Parallel()

		// Create a local repo with a skill
		repoDir := t.TempDir()
		skillDir := filepath.Join(repoDir, "skills", "test-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0o755))

		skillContent := `---
description: Test skill
priority: high
---

# Test Skill

Instructions here.
`
		err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0o644)
		require.NoError(t, err)

		cfg := &config.ConfigV3{
			BaseDir: repoDir, // Use repoDir as baseDir so source resolves correctly
			InstalledSkills: []config.InstalledSkillConfig{
				{
					Name:   "test-skill",
					Source: repoDir,
					Path:   "skills/test-skill",
				},
			},
		}

		skills, err := ResolveInstalledSkills(context.Background(), cfg, "")
		require.NoError(t, err)
		require.Len(t, skills, 1)

		assert.Equal(t, "test-skill", skills[0].Name)
		assert.Contains(t, skills[0].Content, "# Test Skill")
		assert.NotNil(t, skills[0].Metadata)
		assert.Equal(t, "high", skills[0].Metadata.Priority)
	})

	t.Run("uses default path when path is empty", func(t *testing.T) {
		t.Parallel()

		repoDir := t.TempDir()
		// Default path should be skills/<name>
		skillDir := filepath.Join(repoDir, "skills", "my-lib")
		require.NoError(t, os.MkdirAll(skillDir, 0o755))

		err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My Lib\n"), 0o644)
		require.NoError(t, err)

		cfg := &config.ConfigV3{
			BaseDir: repoDir,
			InstalledSkills: []config.InstalledSkillConfig{
				{
					Name:   "my-lib",
					Source: repoDir,
					// Path intentionally empty - should default to skills/my-lib
				},
			},
		}

		skills, err := ResolveInstalledSkills(context.Background(), cfg, "")
		require.NoError(t, err)
		require.Len(t, skills, 1)
		assert.Equal(t, "my-lib", skills[0].Name)
	})

	t.Run("continues on missing skill", func(t *testing.T) {
		t.Parallel()

		repoDir := t.TempDir()

		cfg := &config.ConfigV3{
			BaseDir: repoDir,
			InstalledSkills: []config.InstalledSkillConfig{
				{
					Name:   "nonexistent",
					Source: repoDir,
				},
			},
		}

		skills, err := ResolveInstalledSkills(context.Background(), cfg, "")
		require.NoError(t, err)
		assert.Empty(t, skills)
	})

	t.Run("resolves with local_override", func(t *testing.T) {
		t.Parallel()

		baseDir := t.TempDir()
		overrideDir := t.TempDir()

		// Create skill at override path
		skillDir := filepath.Join(overrideDir, "skills", "override-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0o755))

		err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Override\n"), 0o644)
		require.NoError(t, err)

		cfg := &config.ConfigV3{
			BaseDir: baseDir,
			InstalledSkills: []config.InstalledSkillConfig{
				{
					Name:          "override-skill",
					Source:        "https://github.com/some/repo",
					LocalOverride: overrideDir,
				},
			},
		}

		skills, err := ResolveInstalledSkills(context.Background(), cfg, "")
		require.NoError(t, err)
		require.Len(t, skills, 1)
		assert.Equal(t, "override-skill", skills[0].Name)
		assert.Contains(t, skills[0].Content, "# Override")
	})

	t.Run("skips when local_override path not found", func(t *testing.T) {
		t.Parallel()

		baseDir := t.TempDir()

		cfg := &config.ConfigV3{
			BaseDir: baseDir,
			InstalledSkills: []config.InstalledSkillConfig{
				{
					Name:          "missing-override",
					Source:        "https://github.com/some/repo",
					LocalOverride: "/nonexistent/path",
				},
			},
		}

		skills, err := ResolveInstalledSkills(context.Background(), cfg, "")
		require.NoError(t, err)
		assert.Empty(t, skills)
	})

	t.Run("includes references from local source", func(t *testing.T) {
		t.Parallel()

		repoDir := t.TempDir()
		skillDir := filepath.Join(repoDir, "skills", "ref-skill")
		refsDir := filepath.Join(skillDir, "references")
		require.NoError(t, os.MkdirAll(refsDir, 0o755))

		err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Main\n"), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(refsDir, "api.md"), []byte("API docs.\n"), 0o644)
		require.NoError(t, err)

		cfg := &config.ConfigV3{
			BaseDir: repoDir,
			InstalledSkills: []config.InstalledSkillConfig{
				{
					Name:   "ref-skill",
					Source: repoDir,
				},
			},
		}

		skills, err := ResolveInstalledSkills(context.Background(), cfg, "")
		require.NoError(t, err)
		require.Len(t, skills, 1)
		assert.Contains(t, skills[0].Content, "## Reference: api")
		assert.Contains(t, skills[0].Content, "API docs.")
	})
}

func TestResolveInstalledSkills_Empty(t *testing.T) {
	t.Parallel()

	cfg := &config.ConfigV3{
		BaseDir:         t.TempDir(),
		InstalledSkills: nil,
	}

	skills, err := ResolveInstalledSkills(context.Background(), cfg, "")
	require.NoError(t, err)
	assert.Empty(t, skills)
}
