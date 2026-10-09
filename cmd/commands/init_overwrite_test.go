package commands

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withPipeStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old; _ = r.Close() })
}

func TestShouldOverwriteConfigIgnoresCIEnvironment(t *testing.T) {
	for _, env := range []string{"CI", "NO_INTERACTIVE"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv(env, "true")
			old := autoYes
			autoYes = false
			t.Cleanup(func() { autoYes = old })
			withPipeStdin(t, "y\n")
			if shouldOverwriteConfig(".ai-rulez/") {
				t.Fatalf("%s must not authorize overwriting an existing config directory", env)
			}
		})
	}
}

func TestShouldOverwriteConfigIgnoresYesFlag(t *testing.T) {
	old := autoYes
	autoYes = true
	t.Cleanup(func() { autoYes = old })
	withPipeStdin(t, "")
	if shouldOverwriteConfig(".ai-rulez/") {
		t.Fatal("--yes only skips prompts: it must not authorize replacing a configuration directory")
	}
}

func setForce(t *testing.T, yes, force bool) {
	t.Helper()
	oldYes, oldFrom := autoYes, fromFlag
	autoYes, fromFlag = yes, ""
	setForceFlag(t, force)
	t.Cleanup(func() { autoYes, fromFlag = oldYes, oldFrom })
}

func existingConfig(t *testing.T) (dir, configDir string) {
	t.Helper()
	dir = t.TempDir()
	configDir = filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "rules", "mine.md"), []byte("precious\n"), 0o644))
	chdir(t, dir)
	return dir, configDir
}

func TestInitYesRefusesToReplaceAnExistingDirectory(t *testing.T) {
	_, configDir := existingConfig(t)
	setForce(t, true, false)
	withPipeStdin(t, "")

	err := prepareExistingConfigDir(InitCmd, configDir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
	assert.FileExists(t, filepath.Join(configDir, "rules", "mine.md"))
}

func TestInitForceMovesTheExistingDirectoryToABackup(t *testing.T) {
	dir, configDir := existingConfig(t)
	setForce(t, false, true)

	require.NoError(t, prepareExistingConfigDir(InitCmd, configDir))

	assert.NoDirExists(t, configDir)
	backups, err := filepath.Glob(filepath.Join(dir, ".ai-rulez.bak-*"))
	require.NoError(t, err)
	require.Len(t, backups, 1)
	data, err := os.ReadFile(filepath.Join(backups[0], "rules", "mine.md"))
	require.NoError(t, err)
	assert.Equal(t, "precious\n", string(data))
}

func TestInitWithoutAnExistingDirectoryNeedsNoForce(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	setForce(t, false, false)

	assert.NoError(t, prepareExistingConfigDir(InitCmd, filepath.Join(dir, ".ai-rulez")))
}

func TestBackupConfigDirNeverOverwritesAnEarlierBackup(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	var got []string
	for i := 0; i < 2; i++ {
		configDir := filepath.Join(dir, ".ai-rulez")
		require.NoError(t, os.MkdirAll(configDir, 0o755))
		backup, err := backupConfigDir(configDir, now)
		require.NoError(t, err)
		got = append(got, backup)
	}
	assert.NotEqual(t, got[0], got[1])
	assert.DirExists(t, got[0])
	assert.DirExists(t, got[1])
}

func setForceFlag(t *testing.T, force bool) {
	t.Helper()
	old := initForceFlag(InitCmd)
	require.NoError(t, InitCmd.Flags().Set("force", strconv.FormatBool(force)))
	t.Cleanup(func() { _ = InitCmd.Flags().Set("force", strconv.FormatBool(old)) })
}
