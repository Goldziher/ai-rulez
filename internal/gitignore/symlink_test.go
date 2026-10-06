package gitignore

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
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
	testutil.SymlinkOrSkip(t, target, filepath.Join(dir, ".gitignore"))
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

func TestAnchored_MatchesLikeGitignoreInASubdirectory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tests := []struct {
		name     string
		prefix   string
		pattern  string
		want     string
		matches  []string
		excludes []string
	}{
		{"root project is unchanged", "", "*.local.*", "*.local.*", nil, nil},
		{"slash-less matches at any depth below the project", "sub", "*.local.*", "/sub/**/*.local.*",
			[]string{"sub/a.local.md", "sub/x/y/a.local.md"}, []string{"a.local.md", "other/a.local.md"}},
		{"slash-less directory", "sub", "cache/", "/sub/**/cache/",
			[]string{"sub/cache/f", "sub/x/cache/f"}, []string{"cache/f"}},
		{"middle slash is anchored to the project", "sub", ".claude/rules/*.local.*", "/sub/.claude/rules/*.local.*",
			[]string{"sub/.claude/rules/a.local.md"}, []string{".claude/rules/a.local.md", "sub/x/.claude/rules/a.local.md"}},
		{"leading slash is anchored to the project", "sub", "/AGENTS.md", "/sub/AGENTS.md",
			[]string{"sub/AGENTS.md"}, []string{"AGENTS.md", "sub/x/AGENTS.md"}},
		{"negation is skipped", "sub", "!keep.md", "", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			got := anchored(tt.prefix, tt.pattern)
			require.Equal(t, tt.want, got)
			if got == "" || tt.prefix == "" {
				return
			}
			repo := t.TempDir()
			out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput() //nolint:gosec // test
			require.NoError(t, err, string(out))
			require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git", "info"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(repo, ".git", "info", "exclude"), []byte(got+"\n"), 0o644))

			// Act / Assert
			for _, m := range tt.matches {
				assert.NoError(t, exec.Command("git", "-C", repo, "check-ignore", "--no-index", "-q", m).Run(), m) //nolint:gosec // test
			}
			for _, m := range tt.excludes {
				assert.Error(t, exec.Command("git", "-C", repo, "check-ignore", "--no-index", "-q", m).Run(), m) //nolint:gosec // test
			}
		})
	}
}
