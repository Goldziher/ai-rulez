package crud

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	aiRulezDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(aiRulezDir, 0o755))

	configContent := `version: "3.0"
name: test-project
presets:
  - claude
`
	err := os.WriteFile(filepath.Join(aiRulezDir, "config.yaml"), []byte(configContent), 0o644)
	require.NoError(t, err)

	return dir
}

func TestInstallSkill(t *testing.T) {
	t.Parallel()

	t.Run("installs a skill", func(t *testing.T) {
		t.Parallel()
		dir := setupTestProject(t)

		op, err := NewOperator(dir)
		require.NoError(t, err)

		ctx := context.Background()
		req := &InstallSkillRequest{
			Name:   "test-skill",
			Source: "https://github.com/example/repo",
			Path:   "skills/test-skill",
			Ref:    "main",
		}

		err = op.InstallSkill(ctx, req)
		require.NoError(t, err)

		// Verify it was added
		skills, err := op.ListInstalledSkills(ctx)
		require.NoError(t, err)
		require.Len(t, skills, 1)
		assert.Equal(t, "test-skill", skills[0].Name)
		assert.Equal(t, "https://github.com/example/repo", skills[0].Source)
		assert.Equal(t, "skills/test-skill", skills[0].Path)
		assert.Equal(t, "main", skills[0].Ref)
		assert.Equal(t, "git", skills[0].Type)
	})

	t.Run("rejects duplicate skill name", func(t *testing.T) {
		t.Parallel()
		dir := setupTestProject(t)

		op, err := NewOperator(dir)
		require.NoError(t, err)

		ctx := context.Background()
		req := &InstallSkillRequest{
			Name:   "dup-skill",
			Source: "https://github.com/example/repo",
		}

		err = op.InstallSkill(ctx, req)
		require.NoError(t, err)

		err = op.InstallSkill(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already installed")
	})

	t.Run("rejects empty name", func(t *testing.T) {
		t.Parallel()
		dir := setupTestProject(t)

		op, err := NewOperator(dir)
		require.NoError(t, err)

		ctx := context.Background()
		err = op.InstallSkill(ctx, &InstallSkillRequest{Source: "https://github.com/example/repo"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "skill name is required")
	})

	t.Run("rejects empty source", func(t *testing.T) {
		t.Parallel()
		dir := setupTestProject(t)

		op, err := NewOperator(dir)
		require.NoError(t, err)

		ctx := context.Background()
		err = op.InstallSkill(ctx, &InstallSkillRequest{Name: "test"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "skill source is required")
	})

	t.Run("uses default path when path not specified", func(t *testing.T) {
		t.Parallel()
		dir := setupTestProject(t)

		op, err := NewOperator(dir)
		require.NoError(t, err)

		ctx := context.Background()
		req := &InstallSkillRequest{
			Name:   "my-skill",
			Source: "https://github.com/example/repo",
		}

		err = op.InstallSkill(ctx, req)
		require.NoError(t, err)

		skills, err := op.ListInstalledSkills(ctx)
		require.NoError(t, err)
		require.Len(t, skills, 1)
		// GetPath() should default to "skills/my-skill"
		assert.Equal(t, "skills/my-skill", skills[0].Path)
	})
}

func TestUninstallSkill(t *testing.T) {
	t.Parallel()

	t.Run("removes an installed skill", func(t *testing.T) {
		t.Parallel()
		dir := setupTestProject(t)

		op, err := NewOperator(dir)
		require.NoError(t, err)

		ctx := context.Background()
		req := &InstallSkillRequest{
			Name:   "to-remove",
			Source: "https://github.com/example/repo",
		}

		err = op.InstallSkill(ctx, req)
		require.NoError(t, err)

		err = op.UninstallSkill(ctx, "to-remove")
		require.NoError(t, err)

		skills, err := op.ListInstalledSkills(ctx)
		require.NoError(t, err)
		assert.Empty(t, skills)
	})

	t.Run("returns error for nonexistent skill", func(t *testing.T) {
		t.Parallel()
		dir := setupTestProject(t)

		op, err := NewOperator(dir)
		require.NoError(t, err)

		ctx := context.Background()
		err = op.UninstallSkill(ctx, "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("rejects empty name", func(t *testing.T) {
		t.Parallel()
		dir := setupTestProject(t)

		op, err := NewOperator(dir)
		require.NoError(t, err)

		ctx := context.Background()
		err = op.UninstallSkill(ctx, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "skill name is required")
	})
}

func TestListInstalledSkills(t *testing.T) {
	t.Parallel()

	t.Run("returns empty list when no skills installed", func(t *testing.T) {
		t.Parallel()
		dir := setupTestProject(t)

		op, err := NewOperator(dir)
		require.NoError(t, err)

		ctx := context.Background()
		skills, err := op.ListInstalledSkills(ctx)
		require.NoError(t, err)
		assert.Empty(t, skills)
	})

	t.Run("returns multiple installed skills", func(t *testing.T) {
		t.Parallel()
		dir := setupTestProject(t)

		op, err := NewOperator(dir)
		require.NoError(t, err)

		ctx := context.Background()

		err = op.InstallSkill(ctx, &InstallSkillRequest{
			Name:   "skill-a",
			Source: "https://github.com/example/repo-a",
		})
		require.NoError(t, err)

		err = op.InstallSkill(ctx, &InstallSkillRequest{
			Name:   "skill-b",
			Source: "/local/path/b",
		})
		require.NoError(t, err)

		skills, err := op.ListInstalledSkills(ctx)
		require.NoError(t, err)
		require.Len(t, skills, 2)

		assert.Equal(t, "skill-a", skills[0].Name)
		assert.Equal(t, "git", skills[0].Type)
		assert.Equal(t, "skill-b", skills[1].Name)
		assert.Equal(t, "local", skills[1].Type)
	})
}
