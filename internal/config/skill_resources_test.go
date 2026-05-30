package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSkillResources(t *testing.T) {
	t.Parallel()

	t.Run("returns nothing for empty skill dir", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		resources, err := LoadSkillResources(dir)
		require.NoError(t, err)
		assert.Empty(t, resources)
	})

	t.Run("loads references and extracts description from frontmatter", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		require.NoError(t, os.MkdirAll(filepath.Join(dir, "references"), 0o755))
		ref := "---\ndescription: API endpoints\n---\n\nbody\n"
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "references", "api.md"), []byte(ref), 0o644))

		resources, err := LoadSkillResources(dir)
		require.NoError(t, err)
		require.Len(t, resources, 1)
		assert.Equal(t, SkillKindReferences, resources[0].Kind)
		assert.Equal(t, "references/api.md", resources[0].RelPath)
		assert.Equal(t, "API endpoints", resources[0].Description)
		// Bytes are the original file contents (frontmatter + body) so the
		// generator can write the file out unchanged.
		assert.Equal(t, []byte(ref), resources[0].Content)
	})

	t.Run("falls back to first non-empty line when frontmatter has no description", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		require.NoError(t, os.MkdirAll(filepath.Join(dir, "references"), 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "references", "x.md"),
			[]byte("\n# Heading line\n\nBody.\n"), 0o644))

		resources, err := LoadSkillResources(dir)
		require.NoError(t, err)
		require.Len(t, resources, 1)
		// Heading markers stripped, whitespace trimmed.
		assert.Equal(t, "Heading line", resources[0].Description)
	})

	t.Run("preserves nested paths under references", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		nested := filepath.Join(dir, "references", "api", "v1")
		require.NoError(t, os.MkdirAll(nested, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(nested, "users.md"), []byte("Users API.\n"), 0o644))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "references", "intro.md"), []byte("Intro.\n"), 0o644))

		resources, err := LoadSkillResources(dir)
		require.NoError(t, err)
		require.Len(t, resources, 2)
		// Sorted alphabetically by RelPath; nested files keep forward slashes.
		assert.Equal(t, "references/api/v1/users.md", resources[0].RelPath)
		assert.Equal(t, "references/intro.md", resources[1].RelPath)
	})

	t.Run("loads scripts and assets without parsing descriptions", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		require.NoError(t, os.MkdirAll(filepath.Join(dir, "scripts"), 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "scripts", "run.sh"),
			[]byte("#!/bin/sh\necho hi\n"), 0o755))

		require.NoError(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o755))
		assetBytes := []byte{0xde, 0xad, 0xbe, 0xef}
		require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "blob.bin"), assetBytes, 0o644))

		resources, err := LoadSkillResources(dir)
		require.NoError(t, err)
		require.Len(t, resources, 2)
		assert.Equal(t, SkillKindScripts, resources[0].Kind)
		assert.Empty(t, resources[0].Description)
		// Executable bit on scripts must be preserved through to OutputFile
		// so the agent can invoke them directly.
		if runtime.GOOS != "windows" {
			assert.NotZero(t, resources[0].Mode&0o100,
				"script must keep its executable bit, got mode=%o", resources[0].Mode)
		}
		assert.Equal(t, SkillKindAssets, resources[1].Kind)
		assert.Equal(t, assetBytes, resources[1].Content, "binary asset must round-trip via raw bytes")
	})

	t.Run("ignores non-existent kind directories", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		require.NoError(t, os.MkdirAll(filepath.Join(dir, "references"), 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "references", "a.md"), []byte("a"), 0o644))
		// scripts/ and assets/ deliberately absent.

		resources, err := LoadSkillResources(dir)
		require.NoError(t, err)
		require.Len(t, resources, 1)
		assert.Equal(t, SkillKindReferences, resources[0].Kind)
	})

	t.Run("skips symlinks to prevent exfiltration via untrusted skills", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "references"), 0o755))

		secretDir := t.TempDir()
		secretPath := filepath.Join(secretDir, "secret.txt")
		require.NoError(t, os.WriteFile(secretPath, []byte("classified"), 0o600))

		// Real reference plus a symlink pointing outside the skill tree.
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "references", "real.md"), []byte("real\n"), 0o644))
		require.NoError(t, os.Symlink(secretPath, filepath.Join(dir, "references", "exfil.md")))

		resources, err := LoadSkillResources(dir)
		require.NoError(t, err)
		require.Len(t, resources, 1, "symlink must not be loaded as a resource")
		assert.Equal(t, "references/real.md", resources[0].RelPath)
		// Sanity check: the symlinked content must not appear anywhere.
		for _, r := range resources {
			assert.NotEqual(t, []byte("classified"), r.Content)
		}
	})

	t.Run("refuses to walk a symlinked kind directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Build an attacker-controlled tree, then point references/ at it.
		attackerDir := t.TempDir()
		require.NoError(t, os.WriteFile(
			filepath.Join(attackerDir, "secret.md"), []byte("classified"), 0o644))
		require.NoError(t, os.Symlink(attackerDir, filepath.Join(dir, "references")))

		resources, err := LoadSkillResources(dir)
		require.NoError(t, err)
		// Lstat on the kind dir must catch the symlink so the contents
		// of the attacker's tree are never read.
		assert.Empty(t, resources)
	})

	t.Run("skips symlinked subdirectory inside references/", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "references"), 0o755))

		attackerDir := t.TempDir()
		require.NoError(t, os.WriteFile(
			filepath.Join(attackerDir, "secret.md"), []byte("classified"), 0o644))
		require.NoError(t, os.Symlink(attackerDir, filepath.Join(dir, "references", "evil")))

		resources, err := LoadSkillResources(dir)
		require.NoError(t, err)
		// WalkDir surfaces a symlink-to-directory as a single entry without
		// descending; the per-entry guard then drops it.
		for _, r := range resources {
			assert.NotEqual(t, []byte("classified"), r.Content)
		}
	})

	t.Run("dangling symlinks are dropped without erroring", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "references"), 0o755))
		require.NoError(t, os.Symlink(
			"/nonexistent/path/to/nowhere",
			filepath.Join(dir, "references", "dead.md")))

		resources, err := LoadSkillResources(dir)
		require.NoError(t, err)
		assert.Empty(t, resources)
	})

	t.Run("emits kinds in canonical order: references, scripts, assets", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		for _, kind := range []string{"assets", "scripts", "references"} {
			kindDir := filepath.Join(dir, kind)
			require.NoError(t, os.MkdirAll(kindDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(kindDir, "f.md"), []byte("x"), 0o644))
		}

		resources, err := LoadSkillResources(dir)
		require.NoError(t, err)
		require.Len(t, resources, 3)
		assert.Equal(t, SkillKindReferences, resources[0].Kind)
		assert.Equal(t, SkillKindScripts, resources[1].Kind)
		assert.Equal(t, SkillKindAssets, resources[2].Kind)
	})
}

func TestExtractResourceDescription(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"only-frontmatter-no-desc", "---\npriority: high\n---\n", ""},
		{"frontmatter-desc-wins", "---\ndescription: from-fm\n---\n\n# Heading\n", "from-fm"},
		{"empty-desc-falls-through", "---\ndescription: \n---\n\n# Heading\n", "Heading"},
		{"first-line-fallback", "# Title\n\nBody.\n", "Title"},
		{"strip-multiple-hashes", "### Sub\n\nBody.\n", "Sub"},
		{"skip-empty-lines", "\n\n\n  body line\n", "body line"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, extractResourceDescription([]byte(tc.in)))
		})
	}
}
