package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...) //nolint:gosec // test
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func TestNotARepositoryDegradesGracefully(t *testing.T) {
	dir := t.TempDir()

	tracked, err := TrackedAmong(dir, []string{"a.md"})
	require.NoError(t, err)
	ignored, ignoredErr := IgnoredAmong(dir, []string{"a.md"})
	require.NoError(t, ignoredErr)

	assert.False(t, IsRepo(dir))
	assert.Empty(t, tracked)
	assert.Nil(t, ignored, "nil tells the caller to use its own matcher")
	assert.Empty(t, InfoExcludePath(dir))
	assert.Empty(t, TopLevel(dir))
}

func TestTrackedAmongIsRelativeToTheQueriedDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows file names cannot contain glob metacharacters")
	}
	gitAvailable(t)
	top := t.TempDir()
	runGit(t, top, "init", "-q")
	sub := filepath.Join(top, "pkg")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(top, "root.md"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "AGENTS.md"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "untracked.md"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "we[ir]d*.md"), []byte("x"), 0o600))
	runGit(t, top, "add", "root.md", "pkg/AGENTS.md", "pkg/we[[]ir]d*.md")

	atTop, err := TrackedAmong(top, []string{"root.md", "pkg/AGENTS.md", "pkg/untracked.md"})
	require.NoError(t, err)
	atSub, err := TrackedAmong(sub, []string{"AGENTS.md", "root.md", "we[ir]d*.md", "weird.md"})
	require.NoError(t, err)

	assert.Equal(t, map[string]bool{"root.md": true, "pkg/AGENTS.md": true}, atTop)
	assert.True(t, atSub["AGENTS.md"])
	assert.False(t, atSub["root.md"])
	assert.True(t, atSub["we[ir]d*.md"], "pathspecs are literal")
	assert.False(t, atSub["weird.md"], "a glob in the query must not match other files")
	assert.True(t, IsRepo(sub))
}

func TestIgnoredAmongHonoursNegationAndDoubleStar(t *testing.T) {
	gitAvailable(t)
	top := t.TempDir()
	runGit(t, top, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(top, ".gitignore"), []byte("**/*.gen.md\n!keep.gen.md\n/build/\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(top, ".git", "info"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(top, ".git", "info", "exclude"), []byte("/excluded.md\n"), 0o600))

	ignored, err := IgnoredAmong(top, []string{"a.gen.md", "deep/er/b.gen.md", "keep.gen.md", "build/x.md", "excluded.md", "plain.md"})

	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"a.gen.md": true, "deep/er/b.gen.md": true, "build/x.md": true, "excluded.md": true}, ignored)
}

func TestIgnoredAmongNoneIgnoredIsNotAnError(t *testing.T) {
	gitAvailable(t)
	top := t.TempDir()
	runGit(t, top, "init", "-q")

	ignored, err := IgnoredAmong(top, []string{"a.md"})

	require.NoError(t, err)
	assert.Empty(t, ignored)
}

func TestInfoExcludePathResolvesInLinkedWorktrees(t *testing.T) {
	gitAvailable(t)
	main := t.TempDir()
	runGit(t, main, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(main, "a.txt"), []byte("x"), 0o600))
	runGit(t, main, "add", "a.txt")
	runGit(t, main, "commit", "-q", "-m", "init")
	linked := filepath.Join(t.TempDir(), "wt")
	runGit(t, main, "worktree", "add", "-q", linked, "-b", "feature")

	mainExclude := InfoExcludePath(main)
	linkedExclude := InfoExcludePath(linked)

	require.NotEmpty(t, mainExclude)
	assert.True(t, filepath.IsAbs(linkedExclude))
	assert.Equal(t, Resolve(filepath.Dir(filepath.Dir(mainExclude))), Resolve(filepath.Dir(filepath.Dir(linkedExclude))),
		"linked worktrees share the common info/exclude")
	assert.NotEqual(t, TopLevel(main), TopLevel(linked), "each worktree has its own top level")
	assert.Equal(t, "x/y.md", RepoRelative(TopLevel(linked), filepath.Join(linked, "x", "y.md")))
	assert.Empty(t, RepoRelative(TopLevel(linked), filepath.Join(main, "x.md")))
}

func TestIgnoreRules(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n!keep.log\nbuild/\n"), 0o644))

	rules, err := IgnoreRules(dir, []string{"a.log", "keep.log", "build/x", "plain.md"})

	require.NoError(t, err)
	tests := []struct {
		path                      string
		ignored, negated, matched bool
		pattern                   string
		line                      int
	}{
		{"a.log", true, false, true, "*.log", 1},
		{"keep.log", false, true, true, "!keep.log", 2},
		{"build/x", true, false, true, "build/", 3},
		{"plain.md", false, false, false, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			m := rules[tt.path]
			assert.Equal(t, tt.ignored, m.Ignored())
			assert.Equal(t, tt.negated, m.Negated())
			assert.Equal(t, tt.matched, m.Matched())
			assert.Equal(t, tt.pattern, m.Pattern)
			assert.Equal(t, tt.line, m.Line)
		})
	}
}

func TestIgnoreRulesOutsideRepository(t *testing.T) {
	rules, err := IgnoreRules(t.TempDir(), []string{"a.md"})
	require.NoError(t, err)
	assert.Nil(t, rules)
}

func TestIgnoreRulesMirrored_RewritesWithoutTouchingOriginals(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	gi := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gi, []byte("mine\n# own\nowned\n"), 0o644))
	exclude := InfoExcludePath(dir)
	require.NoError(t, os.MkdirAll(filepath.Dir(exclude), 0o755))
	require.NoError(t, os.WriteFile(exclude, []byte("excl\n"), 0o644))

	rules, err := IgnoreRulesMirrored(dir, []string{"mine", "owned", "excl"}, func(rel, content string) string {
		if rel == ".gitignore" {
			return "mine\n"
		}
		return content
	})

	require.NoError(t, err)
	assert.True(t, rules["mine"].Ignored())
	assert.False(t, rules["owned"].Matched())
	assert.True(t, rules["excl"].Ignored())
	data, readErr := os.ReadFile(gi)
	require.NoError(t, readErr)
	assert.Equal(t, "mine\n# own\nowned\n", string(data))
}

func TestIgnoreRulesMirrored_SkipsSymlinkedIgnoreFiles(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	target := filepath.Join(t.TempDir(), "elsewhere")
	require.NoError(t, os.WriteFile(target, []byte("secret.md\n"), 0o600))
	if err := os.Symlink(target, filepath.Join(sub, ".gitignore")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	rules, err := IgnoreRulesMirrored(dir, []string{"sub/secret.md"}, nil)

	require.NoError(t, err)
	assert.False(t, rules["sub/secret.md"].Matched(), "git does not read a symlinked .gitignore, and neither does the mirror")
}

func TestIgnoreRulesMirrored_SkipsDeviceIgnoreFiles(t *testing.T) {
	gitAvailable(t)
	if _, err := os.Stat("/dev/zero"); err != nil {
		t.Skip("no /dev/zero")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.Symlink("/dev/zero", filepath.Join(sub, ".gitignore")))

	_, err := IgnoreRulesMirrored(dir, []string{"sub/x"}, nil)

	require.NoError(t, err, "a link to an endless file must neither hang nor exhaust memory")
}

func TestIgnoreRulesMirrored_SkipsOversizedIgnoreFiles(t *testing.T) {
	gitAvailable(t)
	old := maxIgnoreFileSize
	maxIgnoreFileSize = 16
	t.Cleanup(func() { maxIgnoreFileSize = old })
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("small\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", ".gitignore"),
		[]byte("big-file-name-that-exceeds-the-cap\n"), 0o600))

	rules, err := IgnoreRulesMirrored(dir, []string{"small", "sub/big-file-name-that-exceeds-the-cap"}, nil)

	require.NoError(t, err)
	assert.True(t, rules["small"].Ignored(), "files within the cap still count")
	assert.False(t, rules["sub/big-file-name-that-exceeds-the-cap"].Matched())
}

func TestTrackedFiles(t *testing.T) {
	t.Run("outside a repository", func(t *testing.T) {
		files, ok, err := TrackedFiles(t.TempDir())
		if err != nil || ok || files != nil {
			t.Fatalf("got %v %v %v, want nil,false,nil", files, ok, err)
		}
	})
	t.Run("reports index entries with modes", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "plain.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil { //nolint:gosec // test needs an executable file
			t.Fatal(err)
		}
		for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}} {
			if _, _, err := run(dir, nil, args...); err != nil {
				t.Skipf("git unavailable: %v", err)
			}
		}
		files, ok, err := TrackedFiles(dir)
		if err != nil || !ok {
			t.Fatalf("TrackedFiles: ok=%v err=%v", ok, err)
		}
		if files["plain.txt"] != 0o100644 {
			t.Errorf("plain.txt mode = %o", files["plain.txt"])
		}
		if runtime.GOOS != "windows" && files["run.sh"] != 0o100755 {
			t.Errorf("run.sh mode = %o", files["run.sh"])
		}
	})
}
