package includes

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckVersion225(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		line    string
		wantErr bool
	}{
		{name: "version 2.39.0", line: "git version 2.39.0", wantErr: false},
		{name: "version 2.24.9 too old", line: "git version 2.24.9", wantErr: true},
		{name: "version 2.25.0 minimum", line: "git version 2.25.0", wantErr: false},
		{name: "not a git version line", line: "not a line", wantErr: true},
		{name: "version 10.0.0 future major", line: "git version 10.0.0", wantErr: false},
		{name: "version 3.0.0", line: "git version 3.0.0", wantErr: false},
		{name: "version 2.24.0", line: "git version 2.24.0", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := checkVersion225(tt.line)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestInjectToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		url   string
		token string
		want  string
	}{
		{
			name:  "HTTPS with token",
			url:   "https://github.com/owner/repo",
			token: "mytoken",
			want:  "https://mytoken:x-oauth-basic@github.com/owner/repo",
		},
		{
			name:  "HTTP with token",
			url:   "http://example.com/repo",
			token: "abc123",
			want:  "http://abc123:x-oauth-basic@example.com/repo",
		},
		{
			name:  "SSH git@ with token unchanged",
			url:   "git@github.com:owner/repo.git",
			token: "mytoken",
			want:  "git@github.com:owner/repo.git",
		},
		{
			name:  "SSH ssh:// with token unchanged",
			url:   "ssh://git@github.com/owner/repo.git",
			token: "mytoken",
			want:  "ssh://git@github.com/owner/repo.git",
		},
		{
			name:  "file:// with token unchanged",
			url:   "file:///tmp/repo",
			token: "mytoken",
			want:  "file:///tmp/repo",
		},
		{
			name:  "HTTPS empty token unchanged",
			url:   "https://github.com/owner/repo",
			token: "",
			want:  "https://github.com/owner/repo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := injectToken(tt.url, tt.token)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReadWriteCacheMeta(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	meta := &CacheMeta{
		RemoteHEADSHA: "abc123def456",
		FetchedAt:     time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		FileHashes: map[string]string{
			"rules/rule1.md": "blake3:deadbeef",
			"context/ctx.md": "blake3:cafebabe",
		},
	}

	err := writeCacheMeta(dir, meta)
	require.NoError(t, err)

	got, err := readCacheMeta(dir)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, meta.RemoteHEADSHA, got.RemoteHEADSHA)
	assert.True(t, meta.FetchedAt.Equal(got.FetchedAt))
	assert.Equal(t, meta.FileHashes, got.FileHashes)
}

func TestReadCacheMetaMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	meta, err := readCacheMeta(dir)
	require.NoError(t, err)
	assert.Nil(t, meta)
}

func TestIsCacheHit(t *testing.T) {
	t.Parallel()

	t.Run("matching SHA", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, writeCacheMeta(dir, &CacheMeta{RemoteHEADSHA: "sha-abc"}))
		assert.True(t, isCacheHit(dir, "sha-abc"))
	})

	t.Run("mismatching SHA", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, writeCacheMeta(dir, &CacheMeta{RemoteHEADSHA: "sha-abc"}))
		assert.False(t, isCacheHit(dir, "sha-xyz"))
	})

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		assert.False(t, isCacheHit(dir, "sha-abc"))
	})
}

func TestComputeFileHashes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	files := map[string]string{
		"alpha.md":   "content alpha",
		"beta.txt":   "content beta",
		"gamma.yaml": "content gamma",
	}
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, cacheMetaFile), []byte(`{}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".hidden"), []byte("hidden"), 0o644))

	hashes, err := computeFileHashes(dir)
	require.NoError(t, err)

	assert.Len(t, hashes, 3, "expected 3 files; cacheMetaFile and hidden file excluded")
	for name := range files {
		hash, ok := hashes[name]
		require.True(t, ok, "expected hash for %s", name)
		assert.True(t, strings.HasPrefix(hash, "blake3:"), "hash should have blake3: prefix, got %s", hash)
	}
	assert.NotContains(t, hashes, cacheMetaFile)
	assert.NotContains(t, hashes, ".hidden")

	t.Run("modified file changes hash", func(t *testing.T) {
		before := hashes["alpha.md"]
		require.NoError(t, os.WriteFile(filepath.Join(dir, "alpha.md"), []byte("different content"), 0o644))

		updated, err := computeFileHashes(dir)
		require.NoError(t, err)
		assert.NotEqual(t, before, updated["alpha.md"])
	})
}

func TestRemoteHEADSHA_LocalRepo(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in PATH")
	}

	repoDir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello"), 0o644))
	run("add", "README.md")
	run("commit", "-m", "initial")

	fileURL := "file://" + repoDir
	ctx := context.Background()

	sha1, err := remoteHEADSHA(ctx, fileURL, "main", "")
	require.NoError(t, err)
	assert.NotEmpty(t, sha1)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "second.md"), []byte("second"), 0o644))
	run("add", "second.md")
	run("commit", "-m", "second")

	sha2, err := remoteHEADSHA(ctx, fileURL, "main", "")
	require.NoError(t, err)
	assert.NotEmpty(t, sha2)
	assert.NotEqual(t, sha1, sha2, "SHA should change after a new commit")
}
