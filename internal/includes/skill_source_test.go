package includes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
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

	t.Run("preserves references as separate resources in alphabetical order", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Main\n\nMain content.\n"), 0o644)
		require.NoError(t, err)

		refsDir := filepath.Join(dir, "references")
		require.NoError(t, os.MkdirAll(refsDir, 0o755))

		err = os.WriteFile(filepath.Join(refsDir, "beta.md"), []byte("Beta reference content.\n"), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(refsDir, "alpha.md"), []byte("Alpha reference content.\n"), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(refsDir, "gamma.md"), []byte("Gamma reference content.\n"), 0o644)
		require.NoError(t, err)

		// Non-md files are kept as references too — the agent might want them.
		// (Description extraction only runs on .md files; non-md content rides
		// through verbatim with empty Description.)
		err = os.WriteFile(filepath.Join(refsDir, "notes.txt"), []byte("ignored by index"), 0o644)
		require.NoError(t, err)

		cf, err := ScanInstalledSkillDir(dir, "with-refs")
		require.NoError(t, err)

		assert.Equal(t, "with-refs", cf.Name)
		// Body must be SKILL.md content alone — no concatenated reference subsections.
		assert.Contains(t, cf.Content, "Main content.")
		assert.NotContains(t, cf.Content, "## Reference:")
		assert.NotContains(t, cf.Content, "Alpha reference content.")

		// Resources must carry the references as separate entries.
		require.Len(t, cf.Resources, 4)
		// All belong to references kind, sorted alphabetically by RelPath.
		assert.Equal(t, "references/alpha.md", cf.Resources[0].RelPath)
		assert.Equal(t, config.SkillKindReferences, cf.Resources[0].Kind)
		assert.Equal(t, []byte("Alpha reference content.\n"), cf.Resources[0].Content)
		assert.Equal(t, "references/beta.md", cf.Resources[1].RelPath)
		assert.Equal(t, "references/gamma.md", cf.Resources[2].RelPath)
		assert.Equal(t, "references/notes.txt", cf.Resources[3].RelPath)
	})

	t.Run("loads scripts and assets alongside references", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Main\n"), 0o644))

		require.NoError(t, os.MkdirAll(filepath.Join(dir, "scripts"), 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "scripts", "run.sh"),
			[]byte("#!/bin/sh\necho hi\n"), 0o755))

		require.NoError(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o755))
		// Binary content (not valid UTF-8) — must round-trip via bytes.
		assetBytes := []byte{0x00, 0xff, 0x10, 0x20, 0x00}
		require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "blob.bin"), assetBytes, 0o644))

		cf, err := ScanInstalledSkillDir(dir, "with-extras")
		require.NoError(t, err)

		require.Len(t, cf.Resources, 2)
		// Order: references kind first (none), then scripts, then assets.
		assert.Equal(t, config.SkillKindScripts, cf.Resources[0].Kind)
		assert.Equal(t, "scripts/run.sh", cf.Resources[0].RelPath)
		assert.Equal(t, config.SkillKindAssets, cf.Resources[1].Kind)
		assert.Equal(t, "assets/blob.bin", cf.Resources[1].RelPath)
		assert.Equal(t, assetBytes, cf.Resources[1].Content)
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
		assert.Empty(t, cf.Resources)
	})

	t.Run("preserves frontmatter on reference files and pulls description", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Main\n"), 0o644)
		require.NoError(t, err)

		refsDir := filepath.Join(dir, "references")
		require.NoError(t, os.MkdirAll(refsDir, 0o755))

		refContent := "---\ndescription: API endpoints reference\npriority: low\n---\n\nReference body.\n"
		err = os.WriteFile(filepath.Join(refsDir, "ref.md"), []byte(refContent), 0o644)
		require.NoError(t, err)

		cf, err := ScanInstalledSkillDir(dir, "fm-refs")
		require.NoError(t, err)

		// Body must NOT contain inlined reference text — references are separate resources.
		assert.NotContains(t, cf.Content, "Reference body.")
		assert.NotContains(t, cf.Content, "priority: low")

		require.Len(t, cf.Resources, 1)
		// The resource keeps the original file bytes (frontmatter included)
		// because the file is emitted verbatim to the rendered skill dir.
		assert.Equal(t, []byte(refContent), cf.Resources[0].Content)
		assert.Equal(t, "API endpoints reference", cf.Resources[0].Description)
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
