package includes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanInstalledSkillDir(t *testing.T) {
	t.Parallel()

	t.Run("reads SKILL.md with frontmatter", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		skillContent := `---
name: test-skill
description: A test skill
priority: high
---

# Test Skill

This is a test skill.
`
		err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillContent), 0o644)
		require.NoError(t, err)

		cf, err := ScanInstalledSkillDir(dir, "test-skill")
		require.NoError(t, err)

		assert.Equal(t, "test-skill", cf.Name)
		assert.Contains(t, cf.Content, "# Test Skill")
		assert.Contains(t, cf.Content, "This is a test skill.")
		assert.NotNil(t, cf.Metadata)
		assert.Equal(t, "high", cf.Metadata.Priority)
	})

	t.Run("reads SKILL.md without frontmatter", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		skillContent := "# Simple Skill\n\nNo frontmatter here.\n"
		err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillContent), 0o644)
		require.NoError(t, err)

		cf, err := ScanInstalledSkillDir(dir, "simple")
		require.NoError(t, err)

		assert.Equal(t, "simple", cf.Name)
		assert.Contains(t, cf.Content, "# Simple Skill")
		assert.Nil(t, cf.Metadata)
	})

	t.Run("includes references in alphabetical order", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Create SKILL.md
		err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Main\n\nMain content.\n"), 0o644)
		require.NoError(t, err)

		// Create references directory with multiple files
		refsDir := filepath.Join(dir, "references")
		require.NoError(t, os.MkdirAll(refsDir, 0o755))

		err = os.WriteFile(filepath.Join(refsDir, "beta.md"), []byte("Beta reference content.\n"), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(refsDir, "alpha.md"), []byte("Alpha reference content.\n"), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(refsDir, "gamma.md"), []byte("Gamma reference content.\n"), 0o644)
		require.NoError(t, err)

		// Non-md file should be ignored
		err = os.WriteFile(filepath.Join(refsDir, "notes.txt"), []byte("ignored"), 0o644)
		require.NoError(t, err)

		cf, err := ScanInstalledSkillDir(dir, "with-refs")
		require.NoError(t, err)

		assert.Equal(t, "with-refs", cf.Name)
		assert.Contains(t, cf.Content, "Main content.")
		assert.Contains(t, cf.Content, "## Reference: alpha")
		assert.Contains(t, cf.Content, "Alpha reference content.")
		assert.Contains(t, cf.Content, "## Reference: beta")
		assert.Contains(t, cf.Content, "Beta reference content.")
		assert.Contains(t, cf.Content, "## Reference: gamma")
		assert.Contains(t, cf.Content, "Gamma reference content.")

		// Verify alphabetical order
		alphaIdx := indexOf(cf.Content, "## Reference: alpha")
		betaIdx := indexOf(cf.Content, "## Reference: beta")
		gammaIdx := indexOf(cf.Content, "## Reference: gamma")
		assert.Less(t, alphaIdx, betaIdx, "alpha should come before beta")
		assert.Less(t, betaIdx, gammaIdx, "beta should come before gamma")
	})

	t.Run("returns error for missing SKILL.md", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		_, err := ScanInstalledSkillDir(dir, "missing")
		assert.Error(t, err)
	})

	t.Run("handles empty references directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Skill\n"), 0o644)
		require.NoError(t, err)

		refsDir := filepath.Join(dir, "references")
		require.NoError(t, os.MkdirAll(refsDir, 0o755))

		cf, err := ScanInstalledSkillDir(dir, "empty-refs")
		require.NoError(t, err)
		assert.Equal(t, "empty-refs", cf.Name)
		assert.Contains(t, cf.Content, "# Skill")
	})

	t.Run("strips frontmatter from reference files", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Main\n"), 0o644)
		require.NoError(t, err)

		refsDir := filepath.Join(dir, "references")
		require.NoError(t, os.MkdirAll(refsDir, 0o755))

		refContent := "---\npriority: low\n---\n\nReference body.\n"
		err = os.WriteFile(filepath.Join(refsDir, "ref.md"), []byte(refContent), 0o644)
		require.NoError(t, err)

		cf, err := ScanInstalledSkillDir(dir, "fm-refs")
		require.NoError(t, err)
		assert.Contains(t, cf.Content, "Reference body.")
		assert.NotContains(t, cf.Content, "priority: low")
	})
}

func TestHasSkillMarker(t *testing.T) {
	t.Parallel()

	t.Run("returns true when SKILL.md exists", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Skill\n"), 0o644)
		require.NoError(t, err)

		assert.True(t, hasSkillMarker(dir))
	})

	t.Run("returns false when SKILL.md is missing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		assert.False(t, hasSkillMarker(dir))
	})
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
