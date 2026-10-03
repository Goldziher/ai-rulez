package gitignore

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func repoWithSymlinkedGitignore(t *testing.T) (dir, target string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir = t.TempDir()
	out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput() //nolint:gosec // test
	require.NoError(t, err, string(out))
	target = filepath.Join(t.TempDir(), "shared-gitignore")
	require.NoError(t, os.WriteFile(target, []byte("# shared\n"), 0o644))
	if err := os.Symlink(target, filepath.Join(dir, ".gitignore")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	return dir, target
}

func TestEnsureEntries_NeverWritesThroughASymlinkedGitignore(t *testing.T) {
	dir, target := repoWithSymlinkedGitignore(t)

	require.NoError(t, EnsureEntries(dir, []string{".ai-rulez/config.local.*", ".ai-rulez/local/"}))

	data, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "# shared\n", string(data), "the link target is untouched")
	out, err := exec.Command("git", "-C", dir, "check-ignore", "--no-index", ".ai-rulez/config.local.toml", ".ai-rulez/local/x.md").CombinedOutput() //nolint:gosec // test
	require.NoError(t, err, string(out))
	assert.Contains(t, string(out), ".ai-rulez/config.local.toml")
}

func TestReplaceViaExclude_ReplacesTheBlockAndRemovesIt(t *testing.T) {
	dir, _ := repoWithSymlinkedGitignore(t)
	require.NoError(t, ReplaceViaExclude(dir, []string{"a.md", "b/"}))
	require.NoError(t, ReplaceViaExclude(dir, []string{"a.md"}))

	exclude, err := os.ReadFile(filepath.Join(dir, ".git", "info", "exclude"))
	require.NoError(t, err)
	assert.Contains(t, string(exclude), "a.md")
	assert.NotContains(t, string(exclude), "b/")

	require.NoError(t, ReplaceViaExclude(dir, nil))
	exclude, err = os.ReadFile(filepath.Join(dir, ".git", "info", "exclude"))
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(exclude), "a.md"))
}

func TestIsSymlink(t *testing.T) {
	dir, _ := repoWithSymlinkedGitignore(t)
	assert.True(t, IsSymlink(dir))
	assert.False(t, IsSymlink(t.TempDir()))
}
