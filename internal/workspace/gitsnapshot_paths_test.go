package workspace_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

func TestReadFileAtReadsOneFileOfARevision(t *testing.T) {
	// Arrange
	dir, first := gitRepo(t)
	tests := []struct {
		name      string
		repoDir   string
		rev       string
		rel       string
		want      string
		wantFound bool
		wantErr   bool
	}{
		{name: "the old content of a changed file", repoDir: dir, rev: first, rel: "a.txt", want: "a at first\n", wantFound: true},
		{name: "a file that no longer exists at head", repoDir: dir, rev: first, rel: "dir/b.txt", want: "b\n", wantFound: true},
		{name: "a file that does not exist at the revision is absent, not an error", repoDir: dir, rev: first, rel: "new.txt"},
		{name: "an unknown revision is an error", repoDir: dir, rev: "no-such-rev", rel: "a.txt", wantErr: true},
		{name: "from a subdirectory the path stays relative to the repository root", repoDir: filepath.Join(dir, "dir"), rev: first, rel: "a.txt", want: "a at first\n", wantFound: true},
		{name: "a symlink entry is followed inside the tree", repoDir: dir, rev: first, rel: "dir/up", want: "a at first\n", wantFound: true},
		{name: "a symlink leaving the tree is an error, not an absent file", repoDir: dir, rev: first, rel: "dir/escape", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			data, found, err := workspace.ReadFileAt(t.Context(), tt.repoDir, tt.rev, tt.rel, nil)

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				assert.False(t, found)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.want, string(data))
		})
	}
}

func TestReadFileAtRefusesAFileOverTheCap(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", ".")
	big := make([]byte, workspace.MaxBaseFileBytes+1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.bin"), big, 0o644)) //nolint:gosec // fixture
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "big")

	// Act
	data, found, err := workspace.ReadFileAt(t.Context(), dir, "HEAD", "big.bin", nil)

	// Assert
	require.Error(t, err, "a file over the cap must not read as a shorter valid file")
	assert.False(t, found)
	assert.Nil(t, data)
}

func TestSnapshotListsLargeDirectoriesOnceAndSorted(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", ".")
	const n = 3000
	for i := range n {
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%05d.txt", n-i)), []byte("x"), 0o644)) //nolint:gosec // fixture
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "many")

	// Act
	snap, err := workspace.GitSnapshot(t.Context(), dir, "HEAD", nil)
	require.NoError(t, err)
	entries, err := snap.ReadDir(".")

	// Assert
	require.NoError(t, err)
	require.Len(t, entries, n)
	for i := 1; i < len(entries); i++ {
		assert.Less(t, entries[i-1].Name(), entries[i].Name())
	}
}

func TestGitSnapshotWithPathsListsOnlyThosePaths(t *testing.T) {
	// Arrange
	dir, first := gitRepo(t)

	// Act
	snap, err := workspace.GitSnapshot(t.Context(), dir, first, nil, "bin")

	// Assert
	require.NoError(t, err)
	_, err = snap.ReadFile("bin/run.sh")
	require.NoError(t, err)
	_, err = snap.ReadFile("a.txt")
	assert.ErrorIs(t, err, fs.ErrNotExist, "a file outside the listed paths is not part of the snapshot")
}
