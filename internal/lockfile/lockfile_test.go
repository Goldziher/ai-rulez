package lockfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveLoad_RoundTripIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	f := &File{}
	f.Set(KindInclude, Entry{Name: "zeta", Source: "https://example.com/z", Commit: "c1", Digest: "sha256:1"})
	f.Set(KindInclude, Entry{Name: "alpha", Source: "https://example.com/a", Ref: "v1", Commit: "c2", Digest: "sha256:2"})
	f.Set(KindSkill, Entry{Name: "s", Source: "https://example.com/s", Path: "skills/s", Commit: "c3", Digest: "sha256:3"})
	require.NoError(t, Save(dir, f))
	first, err := os.ReadFile(Path(dir))
	require.NoError(t, err)

	got, err := Load(dir)
	require.NoError(t, err)
	require.NoError(t, Save(dir, got))
	second, err := os.ReadFile(Path(dir))
	require.NoError(t, err)

	assert.Equal(t, string(first), string(second))
	assert.Equal(t, "alpha", got.Include[0].Name, "entries are sorted by name")
	assert.Equal(t, "c3", got.Find(KindSkill, "s").Commit)
	assert.Nil(t, got.Find(KindSkill, "missing"))
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	got, err := Load(dir)
	require.NoError(t, err)
	assert.Nil(t, got, "a missing lock is not an error")

	require.NoError(t, os.WriteFile(Path(dir), []byte("version = 99\n"), 0o644))
	_, err = Load(dir)
	assert.ErrorContains(t, err, "unsupported lock file version")

	require.NoError(t, os.WriteFile(Path(dir), []byte("not toml ["), 0o644))
	_, err = Load(dir)
	assert.Error(t, err)
}

func TestEntryCovers(t *testing.T) {
	e := &Entry{Source: "https://example.com/r", Path: "p", Ref: "main"}
	tests := []struct {
		name string
		e    *Entry
		w    Want
		want bool
	}{
		{"same", e, Want{Source: "https://example.com/r", Path: "p", Ref: "main"}, true},
		{"ref changed", e, Want{Source: "https://example.com/r", Path: "p", Ref: "dev"}, false},
		{"path changed", e, Want{Source: "https://example.com/r", Path: "q", Ref: "main"}, false},
		{"source changed", e, Want{Source: "https://example.com/x", Path: "p", Ref: "main"}, false},
		{"no entry", nil, Want{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.e.Covers(tt.w))
		})
	}
}

func TestIsFullSHA(t *testing.T) {
	assert.True(t, IsFullSHA("0123456789abcdef0123456789abcdef01234567"))
	assert.False(t, IsFullSHA("main"))
	assert.False(t, IsFullSHA("0123456789ABCDEF0123456789abcdef01234567"))
	assert.False(t, IsFullSHA("0123456"))
}

func TestDigestDir(t *testing.T) {
	write := func(dir, rel, body string, mode os.FileMode) {
		p := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), mode))
		require.NoError(t, os.Chmod(p, mode))
	}
	a, b := t.TempDir(), t.TempDir()
	for _, d := range []string{a, b} {
		write(d, "x/one.md", "1", 0o644)
		write(d, "two.md", "2", 0o644)
	}
	write(b, ".git/HEAD", "ref", 0o644)
	write(b, ".cache_meta.json", "{}", 0o644)
	da, err := DigestDir(a)
	require.NoError(t, err)
	db, err := DigestDir(b)
	require.NoError(t, err)
	assert.Equal(t, da, db, "VCS metadata and cache bookkeeping do not count")

	write(b, "two.md", "changed", 0o644)
	dc, err := DigestDir(b)
	require.NoError(t, err)
	assert.NotEqual(t, da, dc, "content changes the digest")

	write(b, "two.md", "2", 0o755)
	dd, err := DigestDir(b)
	require.NoError(t, err)
	assert.NotEqual(t, da, dd, "the executable bit changes the digest")
}
