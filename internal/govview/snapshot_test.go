package govview

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func snapGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	cmd := gitutil.Command(context.Background(), dir, append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
	cmd.Env = append(cmd.Env, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

func snapWrite(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// snapRepo makes a repository with two commits that differ in one rule, plus an
// untracked machine-local file.
func snapRepo(t *testing.T) (dir, first, second string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir = t.TempDir()
	snapGit(t, dir, "init", "-q", "-b", "main")
	snapWrite(t, filepath.Join(dir, "svc", ".ai-rulez", "rules", "a.md"), "one\n")
	snapWrite(t, filepath.Join(dir, "svc", ".ai-rulez", "config.toml"), "name = \"x\"\n")
	snapWrite(t, filepath.Join(dir, "outside.txt"), "not config\n")
	snapGit(t, dir, "add", "-A")
	snapGit(t, dir, "commit", "-q", "-m", "one")
	first = trimNL(snapGit(t, dir, "rev-parse", "HEAD"))
	snapWrite(t, filepath.Join(dir, "svc", ".ai-rulez", "rules", "a.md"), "two\n")
	snapWrite(t, filepath.Join(dir, "svc", ".ai-rulez", "rules", "b.md"), "new\n")
	snapGit(t, dir, "add", "-A")
	snapGit(t, dir, "commit", "-q", "-m", "two")
	second = trimNL(snapGit(t, dir, "rev-parse", "HEAD"))
	snapWrite(t, filepath.Join(dir, "svc", ".ai-rulez", "local", "private.md"), "untracked\n")
	return dir, first, second
}

func trimNL(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func TestExtractRevision(t *testing.T) {
	// Arrange
	dir, first, second := snapRepo(t)
	tests := []struct {
		name      string
		rev       string
		wantA     string
		wantB     bool
		wantFiles int
		commit    string
	}{
		{"first commit by sha", first, "one\n", false, 2, first},
		{"head", "HEAD", "two\n", true, 3, second},
		{"relative rev", "HEAD~1", "one\n", false, 2, first},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest := t.TempDir()

			// Act
			snap, err := ExtractRevision(context.Background(), dir, tt.rev, "svc/.ai-rulez", dest)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.commit, snap.Commit)
			assert.Equal(t, tt.wantFiles, snap.Files)
			got, readErr := os.ReadFile(filepath.Join(dest, "svc", ".ai-rulez", "rules", "a.md"))
			require.NoError(t, readErr)
			assert.Equal(t, tt.wantA, string(got))
			_, bErr := os.Stat(filepath.Join(dest, "svc", ".ai-rulez", "rules", "b.md"))
			assert.Equal(t, tt.wantB, bErr == nil)
			assert.NoFileExists(t, filepath.Join(dest, "svc", ".ai-rulez", "local", "private.md"), "an untracked file is not part of a revision")
			assert.NoFileExists(t, filepath.Join(dest, "outside.txt"))
		})
	}
}

func TestExtractRevision_Refusals(t *testing.T) {
	// Arrange
	dir, _, _ := snapRepo(t)
	tests := []struct {
		name string
		dir  string
		rev  string
		rel  string
		want string
	}{
		{"unknown revision", dir, "no-such-rev", "svc/.ai-rulez", "does not exist"},
		{"option as revision", dir, "--output=/tmp/x", "svc/.ai-rulez", "git option"},
		{"missing path at the revision", dir, "HEAD", "nope/.ai-rulez", "no files"},
		{"not a repository", t.TempDir(), "HEAD", "svc/.ai-rulez", "not inside a git work tree"},
		{"escaping path", dir, "HEAD", "../outside", "invalid path"},
		{"empty path", dir, "HEAD", ".", "invalid path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := ExtractRevision(context.Background(), tt.dir, tt.rev, tt.rel, t.TempDir())

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestExtractRevision_SymlinksAreReportedNotMaterialised(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	snapGit(t, dir, "init", "-q", "-b", "main")
	snapWrite(t, filepath.Join(dir, ".ai-rulez", "rules", "a.md"), "one\n")
	testutil.SymlinkOrSkip(t, "/etc/passwd", filepath.Join(dir, ".ai-rulez", "rules", "link.md"))
	snapGit(t, dir, "add", "-A")
	snapGit(t, dir, "commit", "-q", "-m", "one")
	dest := t.TempDir()

	// Act
	snap, err := ExtractRevision(context.Background(), dir, "HEAD", ".ai-rulez", dest)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{".ai-rulez/rules/link.md"}, snap.Symlinks)
	_, statErr := os.Lstat(filepath.Join(dest, ".ai-rulez", "rules", "link.md"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestExtractRevisionAll(t *testing.T) {
	// Arrange
	dir, first, _ := snapRepo(t)
	dest := t.TempDir()

	// Act
	snap, err := ExtractRevisionAll(context.Background(), dir, first, dest)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, first, snap.Commit)
	assert.Equal(t, 3, snap.Files)
	got, readErr := os.ReadFile(filepath.Join(dest, "svc", ".ai-rulez", "rules", "a.md"))
	require.NoError(t, readErr)
	assert.Equal(t, "one\n", string(got))
	assert.FileExists(t, filepath.Join(dest, "outside.txt"), "the whole repository, not one directory")
	assert.NoFileExists(t, filepath.Join(dest, "svc", ".ai-rulez", "local", "private.md"))
}
