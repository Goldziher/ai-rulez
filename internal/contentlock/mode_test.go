package contentlock

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/gitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileModeForWindowsUsesTheGitIndexNotTheFilesystem(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "run.sh")
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"), 0o644))
	info, err := os.Stat(p)
	require.NoError(t, err)

	exec100755 := func(string) (string, bool) { return ModeExecutable, true }
	noIndex := func(string) (string, bool) { return "", false }

	assert.Equal(t, ModeExecutable, fileModeFor("windows", p, info, exec100755), "the index says executable")
	assert.Equal(t, ModeRegular, fileModeFor("windows", p, info, noIndex), "no index: regular, never guessed from the filesystem")
	assert.Equal(t, ModeRegular, fileModeFor("linux", p, info, exec100755), "unix reads the file, not the index")

	require.NoError(t, os.Chmod(p, 0o755))
	info, err = os.Stat(p)
	require.NoError(t, err)
	assert.Equal(t, ModeExecutable, fileModeFor("linux", p, info, noIndex))
	assert.Equal(t, ModeRegular, fileModeFor("windows", p, info, noIndex), "windows ignores the file's own bits so every OS agrees on the same checkout")
}

func TestGitIndexMode(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := gitutil.CommandNoContext(dir, args...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.sh"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x"), 0o644))
	run("add", "a.sh", "b.txt")
	run("update-index", "--chmod=+x", "a.sh")

	mode, ok := gitIndexMode(filepath.Join(dir, "a.sh"))
	assert.True(t, ok)
	assert.Equal(t, ModeExecutable, mode)
	mode, ok = gitIndexMode(filepath.Join(dir, "b.txt"))
	assert.True(t, ok)
	assert.Equal(t, ModeRegular, mode)
	_, ok = gitIndexMode(filepath.Join(dir, "untracked.sh"))
	assert.False(t, ok)
}
