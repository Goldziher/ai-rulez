package workspace_test

import (
	"io/fs"
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
		name    string
		repoDir string
		rev     string
		rel     string
		want    string
		wantOK  bool
	}{
		{name: "the old content of a changed file", repoDir: dir, rev: first, rel: "a.txt", want: "a at first\n", wantOK: true},
		{name: "a file that no longer exists at head", repoDir: dir, rev: first, rel: "dir/b.txt", want: "b\n", wantOK: true},
		{name: "a file that does not exist at the revision", repoDir: dir, rev: first, rel: "new.txt"},
		{name: "an unknown revision", repoDir: dir, rev: "no-such-rev", rel: "a.txt"},
		{name: "from a subdirectory the path stays relative to the repository root", repoDir: filepath.Join(dir, "dir"), rev: first, rel: "a.txt", want: "a at first\n", wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			data, ok := workspace.ReadFileAt(t.Context(), tt.repoDir, tt.rev, tt.rel, nil)

			// Assert
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, string(data))
		})
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
