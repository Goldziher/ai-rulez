package gitutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)
	out, err := CommandNoContext(dir, full...).CombinedOutput()
	require.NoError(t, err, string(out))
}

func writeIn(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func diffFixture(t *testing.T) string {
	t.Helper()
	gitAvailable(t)
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	writeIn(t, dir, "a.txt", "one\ntwo\nthree\n")
	writeIn(t, dir, "gone.txt", "bye\n")
	writeIn(t, dir, "old name.txt", "line1\nline2\nline3\nline4\nline5\n")
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "base")
	gitIn(t, dir, "checkout", "-q", "-b", "topic")
	return dir
}

func byPath(changes []Change) map[string]Change {
	m := map[string]Change{}
	for _, c := range changes {
		m[c.Path] = c
	}
	return m
}

func TestChangesSince(t *testing.T) {
	// Arrange
	dir := diffFixture(t)
	writeIn(t, dir, "a.txt", "one\ntwo\nTHREE\nfour\n")
	gitIn(t, dir, "rm", "-q", "gone.txt")
	gitIn(t, dir, "mv", "old name.txt", "new name.txt")
	writeIn(t, dir, "new name.txt", "line1\nline2\nline3\nline4\nline5\nline6\n")
	writeIn(t, dir, "fresh.txt", "x\ny\n")
	gitIn(t, dir, "add", "a.txt", "new name.txt")
	gitIn(t, dir, "commit", "-q", "-m", "topic")
	writeIn(t, dir, "untracked.txt", "u\n")
	writeIn(t, dir, "sub/dir/ignored.log", "x\n")
	writeIn(t, dir, ".gitignore", "*.log\n")

	// Act
	changes, err := ChangesSince(dir, "main")

	// Assert
	require.NoError(t, err)
	got := byPath(changes)
	assert.Equal(t, []LineRange{{3, 4}}, got["a.txt"].Added)
	assert.Equal(t, byte('D'), got["gone.txt"].Status)
	renamed := got["new name.txt"]
	assert.Equal(t, byte('R'), renamed.Status)
	assert.Equal(t, "old name.txt", renamed.OldPath)
	assert.Equal(t, []LineRange{{6, 6}}, renamed.Added)
	assert.True(t, got["fresh.txt"].AllAdded)
	assert.True(t, got["untracked.txt"].AllAdded)
	assert.NotContains(t, got, "sub/dir/ignored.log")
}

func TestChangesSinceUnknownBaseIsAnError(t *testing.T) {
	dir := diffFixture(t)

	_, err := ChangesSince(dir, "no-such-ref")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no-such-ref")
}

func TestChangesSinceRejectsOptionLikeRev(t *testing.T) {
	dir := diffFixture(t)

	_, err := ChangesSince(dir, "--output=x")

	require.Error(t, err)
}

func TestStagedChanges(t *testing.T) {
	dir := diffFixture(t)
	writeIn(t, dir, "a.txt", "one\ntwo\nthree\nfour\n")
	writeIn(t, dir, "b.txt", "b\n")
	gitIn(t, dir, "add", "a.txt", "b.txt")
	writeIn(t, dir, "c.txt", "unstaged\n")

	changes, err := StagedChanges(dir)

	require.NoError(t, err)
	got := byPath(changes)
	assert.Len(t, got, 2)
	assert.Equal(t, []LineRange{{4, 4}}, got["a.txt"].Added)
	assert.True(t, got["b.txt"].AllAdded)
}

func TestChangesAreRelativeToTheQueryDirectory(t *testing.T) {
	dir := diffFixture(t)
	writeIn(t, dir, "proj/x.txt", "x\n")
	writeIn(t, dir, "outside.txt", "o\n")

	changes, err := ChangesSince(filepath.Join(dir, "proj"), "main")

	require.NoError(t, err)
	got := byPath(changes)
	assert.Contains(t, got, "x.txt")
	assert.NotContains(t, got, "outside.txt")
}

func TestListFilesIncludesUntrackedButNotIgnored(t *testing.T) {
	dir := diffFixture(t)
	writeIn(t, dir, ".gitignore", "*.log\n")
	writeIn(t, dir, "u.txt", "u\n")
	writeIn(t, dir, "x.log", "x\n")

	files, ok, err := ListFiles(dir)

	require.NoError(t, err)
	require.True(t, ok)
	assert.Contains(t, files, "a.txt")
	assert.Contains(t, files, "u.txt")
	assert.NotContains(t, files, "x.log")
}

func TestListFilesOutsideRepository(t *testing.T) {
	_, ok, err := ListFiles(t.TempDir())

	require.NoError(t, err)
	assert.False(t, ok)
}

func TestChangesIgnoreDiffPrefixConfig(t *testing.T) {
	for _, cfg := range [][]string{{"diff.noprefix", "true"}, {"diff.mnemonicPrefix", "true"}, {"diff.srcPrefix", "x/"}} {
		t.Run(cfg[0], func(t *testing.T) {
			// Arrange
			dir := diffFixture(t)
			gitIn(t, dir, "config", cfg[0], cfg[1])
			writeIn(t, dir, "a.txt", "one\ntwo\nthree\nfour\n")
			gitIn(t, dir, "add", "a.txt")

			// Act
			staged, err := StagedChanges(dir)
			require.NoError(t, err)
			gitIn(t, dir, "commit", "-q", "-m", "x")
			writeIn(t, dir, "a.txt", "one\ntwo\nthree\nfour\nfive\n")
			since, err2 := ChangesSince(dir, "main")

			// Assert
			require.NoError(t, err2)
			assert.Equal(t, []LineRange{{4, 4}}, byPath(staged)["a.txt"].Added)
			assert.Equal(t, []LineRange{{4, 5}}, byPath(since)["a.txt"].Added)
		})
	}
}

func TestFirstParentCommits(t *testing.T) {
	// Arrange: base, a merged topic (merge commit) and a plain commit on main.
	dir := diffFixture(t)
	writeIn(t, dir, "b.txt", "b\n")
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "topic work")
	gitIn(t, dir, "checkout", "-q", "main")
	gitIn(t, dir, "merge", "-q", "--no-ff", "-m", "merge topic", "topic")
	writeIn(t, dir, "c.txt", "c\n")
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "direct")

	tests := []struct {
		name string
		n    int
		want int
	}{
		{"all of them", 10, 3},
		{"the newest two", 2, 2},
		{"none", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			commits, err := FirstParentCommits(dir, tt.n)

			// Assert
			require.NoError(t, err)
			assert.Len(t, commits, tt.want)
		})
	}
	commits, err := FirstParentCommits(dir, 10)
	require.NoError(t, err)
	assert.Equal(t, "direct", commits[0].Subject)
	assert.Equal(t, "merge topic", commits[1].Subject)
	assert.NotEmpty(t, commits[1].Parent, "a merge commit has a first parent")
	assert.Empty(t, commits[2].Parent, "the root commit has none")
}

func TestChangesBetween(t *testing.T) {
	// Arrange
	dir := diffFixture(t)
	gitIn(t, dir, "checkout", "-q", "main")
	base := revParse(t, dir, "HEAD")
	writeIn(t, dir, "a.txt", "one\ntwo\nTHREE\nfour\n")
	writeIn(t, dir, "fresh.txt", "x\n")
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "change")
	head := revParse(t, dir, "HEAD")
	writeIn(t, dir, "later.txt", "not in the range\n")

	// Act
	changes, err := ChangesBetween(dir, base, head)

	// Assert
	require.NoError(t, err)
	got := byPath(changes)
	assert.Equal(t, []LineRange{{3, 4}}, got["a.txt"].Added)
	assert.True(t, got["fresh.txt"].AllAdded)
	assert.NotContains(t, got, "later.txt", "the working tree is not part of a commit range")
}

func TestChangesBetweenRefusesAnOptionLookingRevision(t *testing.T) {
	dir := diffFixture(t)

	_, err := ChangesBetween(dir, "--output=x", "HEAD")

	require.Error(t, err)
}

func revParse(t *testing.T, dir, rev string) string {
	t.Helper()
	out, err := CommandNoContext(dir, "rev-parse", rev).Output()
	require.NoError(t, err)
	return string(out[:len(out)-1])
}
