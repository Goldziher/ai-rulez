package contentlock

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// goldenSkillDigest is the vector of docs/lockfile.md for the fixture skill below.
const goldenSkillDigest = "sha256:262d721306783b4c3b554a1345253c266f6f991733dad6a347e1b55d5e57ac05"

func writeGoldenSkill(t *testing.T, dir string) {
	t.Helper()
	files := []struct {
		rel  string
		data []byte
		perm os.FileMode
	}{
		{"SKILL.md", []byte("---\nname: deploy\n---\nDeploy.\n"), 0o644},
		{"scripts/run.sh", []byte("#!/bin/sh\necho hi\n"), 0o755},
		{"assets/logo.bin", []byte{0x00, 0x01, 0x0d, 0x0a, 0x02}, 0o644},
	}
	for _, f := range files {
		abs := filepath.Join(dir, filepath.FromSlash(f.rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, f.data, f.perm))
	}
}

func TestSkillDirDigest(t *testing.T) {
	t.Run("should equal the documented golden vector", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the executable bit comes from git on Windows")
		}
		dir := filepath.Join(t.TempDir(), "deploy")
		writeGoldenSkill(t, dir)

		got, err := SkillDirDigest(dir)

		require.NoError(t, err)
		assert.Equal(t, goldenSkillDigest, got)
	})

	t.Run("should ignore the top-level evals directory", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "deploy")
		writeGoldenSkill(t, dir)
		before, err := SkillDirDigest(dir)
		require.NoError(t, err)

		require.NoError(t, os.MkdirAll(filepath.Join(dir, "evals", "fixtures"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "evals", "case.md"), []byte("case\n"), 0o644))
		after, err := SkillDirDigest(dir)

		require.NoError(t, err)
		assert.Equal(t, before, after)
	})

	t.Run("should change when a resource changes", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "deploy")
		writeGoldenSkill(t, dir)
		before, err := SkillDirDigest(dir)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "logo.bin"), []byte{0x09}, 0o644))
		after, err := SkillDirDigest(dir)

		require.NoError(t, err)
		assert.NotEqual(t, before, after)
	})

	t.Run("should equal the digest the lock pins for the same skill", func(t *testing.T) {
		f := newFixture(t)
		f.skill("", "deploy", map[string]string{"SKILL.md": "---\nname: deploy\n---\nDeploy.\n", "references/api.md": "api\n", "scripts/run.sh": "echo hi\n"})
		f.write("skills/deploy/evals/case.md", []byte("case\n"), 0o644)
		pinned := f.items()["skill:/deploy"]
		require.NotEmpty(t, pinned)

		got, err := SkillDirDigest(filepath.Join(f.cfg.ConfigDir, "domains", "", "skills", "deploy"))

		require.NoError(t, err)
		assert.Equal(t, pinned, got)
	})

	t.Run("should refuse a directory without SKILL.md", func(t *testing.T) {
		_, err := SkillDirDigest(t.TempDir())

		assert.Error(t, err)
	})
}

func TestSkillDigestSchemeIsTheLockLabel(t *testing.T) {
	assert.Equal(t, label(KindSkill), SkillDigestScheme)
}

func TestSkillDigest_ReadErrorIsNotHiddenByTheInMemoryContent(t *testing.T) {
	dir := t.TempDir()
	// A SKILL.md that exists but cannot be read as a file: a directory stands in
	// for a permission or I/O failure on every platform.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "SKILL.md"), 0o750))
	skill := &config.ContentFile{Path: filepath.Join(dir, "SKILL.md"), Content: "# in memory\n"}

	_, err := SkillDigest(skill, dir)

	require.Error(t, err, "a digest of stale in-memory content would be silently wrong")
}

func TestSkillDigest_MissingFileFallsBackToInMemoryContent(t *testing.T) {
	skill := &config.ContentFile{Path: filepath.Join(t.TempDir(), "absent", "SKILL.md"), Content: "# builtin\n"}

	digest, err := SkillDigest(skill, t.TempDir())

	require.NoError(t, err)
	assert.NotEmpty(t, digest)
}
