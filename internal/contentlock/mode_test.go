package contentlock

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

func TestModeResolver_WindowsReadsTheGitIndexOncePerTree(t *testing.T) {
	// Arrange
	root := t.TempDir()
	exe := filepath.Join(root, "run.sh")
	plain := filepath.Join(root, "sub", "a.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(plain), 0o755))
	require.NoError(t, os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o644))
	require.NoError(t, os.WriteFile(plain, []byte("x"), 0o644))
	info, err := os.Stat(exe)
	require.NoError(t, err)
	calls := 0
	r := &modeResolver{goos: "windows", trees: map[string]map[string]uint32{},
		tracked: func(string) (map[string]uint32, bool, error) {
			calls++
			return map[string]uint32{"run.sh": 0o100755, "sub/a.md": 0o100644}, true, nil
		}}

	// Act
	gotExe := r.mode(root, exe, info)
	gotPlain := r.mode(root, plain, info)
	gotOutside := r.mode(root, filepath.Join(filepath.Dir(root), "other.sh"), info)

	// Assert
	assert.Equal(t, 1, calls, "one git call for the tree, not one per file")
	assert.Equal(t, ModeExecutable, gotExe, "the index says executable")
	assert.Equal(t, ModeRegular, gotPlain)
	assert.Equal(t, ModeRegular, gotOutside, "a file outside the tree is never guessed")
}

func TestModeResolver_NoIndexIsRegularOnWindowsAndUnixReadsTheFile(t *testing.T) {
	// Arrange: the file's own bits are 0755 (described in memory, because a
	// Windows file system has no executable bit to write).
	root := t.TempDir()
	p := filepath.Join(root, "run.sh")
	info, err := fstest.MapFS{"run.sh": {Data: []byte("#!/bin/sh\n"), Mode: 0o755}}.Stat("run.sh")
	require.NoError(t, err)
	noIndex := func(string) (map[string]uint32, bool, error) { return nil, false, nil }

	// Act
	windows := (&modeResolver{goos: "windows", tracked: noIndex, trees: map[string]map[string]uint32{}}).mode(root, p, info)
	unix := (&modeResolver{goos: "linux", tracked: noIndex, trees: map[string]map[string]uint32{}}).mode(root, p, info)

	// Assert
	assert.Equal(t, ModeRegular, windows, "windows ignores the file's own bits so every OS agrees on the same checkout")
	assert.Equal(t, ModeExecutable, unix)
}

func TestModeResolver_WindowsWithARealRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		out, err := gitutil.CommandNoContext(dir, args...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.sh"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x"), 0o644))
	run("add", "a.sh", "b.txt")
	run("update-index", "--chmod=+x", "a.sh")
	r := &modeResolver{goos: "windows", tracked: gitutil.TrackedFiles, trees: map[string]map[string]uint32{}}
	info, err := os.Stat(filepath.Join(dir, "a.sh"))
	require.NoError(t, err)

	assert.Equal(t, ModeExecutable, r.mode(dir, filepath.Join(dir, "a.sh"), info))
	assert.Equal(t, ModeRegular, r.mode(dir, filepath.Join(dir, "b.txt"), info))
	assert.Equal(t, ModeRegular, r.mode(dir, filepath.Join(dir, "untracked.sh"), info))
}
