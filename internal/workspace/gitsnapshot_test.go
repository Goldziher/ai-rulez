package workspace_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	all := append([]string{"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
	cmd := gitutil.CommandNoContext(dir, all...)
	cmd.Env = append(gitutil.Env(nil), "HOME="+t.TempDir(), "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

// gitRepo commits a small tree and then changes the work tree, so a snapshot of
// the first commit differs from the disk.
func gitRepo(t *testing.T) (dir, first string) {
	t.Helper()
	dir = t.TempDir()
	runGit(t, dir, "init", "-q", ".")
	write := func(name, content string, mode os.FileMode) {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), mode)) //nolint:gosec // fixture
	}
	write("a.txt", "a at first\n", 0o644)
	write("dir/b.txt", "b\n", 0o644)
	write("bin/run.sh", "#!/bin/sh\n", 0o755)
	testutil.SymlinkOrSkip(t, "../a.txt", filepath.Join(dir, "dir", "up"))
	testutil.SymlinkOrSkip(t, "../../outside", filepath.Join(dir, "dir", "escape"))
	runGit(t, dir, "add", "-A")
	// Record the executable bit in the index: on Windows git does not read it from the file system.
	runGit(t, dir, "update-index", "--chmod=+x", "bin/run.sh")
	runGit(t, dir, "commit", "-q", "-m", "first")
	first = trimmed(runGit(t, dir, "rev-parse", "HEAD"))

	write("a.txt", "a changed later\n", 0o644)
	write("new.txt", "only in the second commit\n", 0o644)
	require.NoError(t, os.Remove(filepath.Join(dir, "dir", "b.txt")))
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "second")
	write("a.txt", "a edited on disk\n", 0o644)
	return dir, first
}

func trimmed(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func TestGitSnapshotReadsTheCommitNotTheDisk(t *testing.T) {
	// Arrange
	dir, first := gitRepo(t)

	// Act
	snap, err := workspace.GitSnapshot(t.Context(), dir, first, nil)
	require.NoError(t, err)

	// Assert: the old content, files that later vanished, none of what came after.
	data, err := snap.ReadFile("a.txt")
	require.NoError(t, err)
	assert.Equal(t, "a at first\n", string(data))
	data, err = snap.ReadFile("dir/b.txt")
	require.NoError(t, err)
	assert.Equal(t, "b\n", string(data))
	_, err = snap.ReadFile("new.txt")
	assert.ErrorIs(t, err, fs.ErrNotExist)
	assert.Equal(t, first, snap.Commit())
	assert.Equal(t, dir, snap.Root())

	entries, err := snap.ReadDir("dir")
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"b.txt", "escape", "up"}, names)

	info, err := snap.Lstat("bin/run.sh")
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0o100, "the executable bit comes from the tree entry")
}

func TestGitSnapshotSymlinksResolveInsideTheTreeOnly(t *testing.T) {
	// Arrange
	dir, first := gitRepo(t)
	snap, err := workspace.GitSnapshot(t.Context(), dir, first, nil)
	require.NoError(t, err)

	// Act
	info, lerr := snap.Lstat("dir/up")
	target, rerr := snap.ReadLink("dir/up")
	through, terr := snap.ReadFile("dir/up")
	_, eerr := snap.ReadFile("dir/escape")

	// Assert
	require.NoError(t, lerr)
	assert.NotZero(t, info.Mode()&fs.ModeSymlink)
	require.NoError(t, rerr)
	assert.Equal(t, "../a.txt", target)
	require.NoError(t, terr)
	assert.Equal(t, "a at first\n", string(through))
	require.Error(t, eerr)
	assert.ErrorIs(t, eerr, workspace.ErrOutside)
}

func TestGitSnapshotRejectsWhatItCannotRead(t *testing.T) {
	dir, _ := gitRepo(t)
	tests := []struct {
		name   string
		rev    string
		runner runner.Runner
	}{
		{name: "an unknown revision", rev: "no-such-branch"},
		{name: "an option as a revision", rev: "--upload-pack=x"},
		{name: "an empty revision", rev: " "},
		{name: "a runner that denies git", rev: "HEAD", runner: runner.Deny{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := workspace.GitSnapshot(t.Context(), dir, tt.rev, tt.runner)
			require.Error(t, err)
		})
	}
}

func TestGitSnapshotAndMemAgreeOnTheSameTree(t *testing.T) {
	// Arrange: the files of the first commit, in memory and in a snapshot.
	dir, first := gitRepo(t)
	snap, err := workspace.GitSnapshot(t.Context(), dir, first, nil)
	require.NoError(t, err)
	mem := workspace.NewMem(dir)
	mem.Set("a.txt", "a at first\n", 0o644)
	mem.Set("dir/b.txt", "b\n", 0o644)
	mem.Set("bin/run.sh", "#!/bin/sh\n", 0o755)
	mem.Symlink("dir/up", "../a.txt")
	mem.Symlink("dir/escape", "../../outside")

	// Act and assert: every read answers the same.
	walk := func(ws workspace.Workspace) map[string]string {
		out := map[string]string{}
		require.NoError(t, fs.WalkDir(ws, ".", func(p string, d fs.DirEntry, err error) error {
			require.NoError(t, err)
			if d.IsDir() {
				return nil
			}
			info, err := ws.Lstat(p)
			require.NoError(t, err)
			if info.Mode()&fs.ModeSymlink != 0 {
				target, err := ws.ReadLink(p)
				require.NoError(t, err)
				out[p] = "-> " + target
				return nil
			}
			data, err := ws.ReadFile(p)
			require.NoError(t, err)
			out[p] = string(data)
			return nil
		}))
		return out
	}
	assert.Equal(t, walk(mem), walk(snap))
}

func TestGitSnapshotFailureIsNotAFilesystemError(t *testing.T) {
	_, err := workspace.GitSnapshot(t.Context(), t.TempDir(), "HEAD", nil)
	require.Error(t, err)
	assert.False(t, errors.Is(err, fs.ErrNotExist), "a missing repository is reported as such, not as a missing file")
}
