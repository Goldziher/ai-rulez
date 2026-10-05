package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withUserHome points user scope at a temporary home directory holding a minimal
// user config, and restores every package flag the test touches.
func withUserHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	oldHome, oldCfg, oldDry, oldYes, oldProfile := userHomeDir, cfgFile, dryRun, assumeYes, profile
	userHomeDir = func() (string, error) { return home, nil }
	cfgFile, dryRun, assumeYes, profile = "", false, false, ""
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Cleanup(func() { userHomeDir, cfgFile, dryRun, assumeYes, profile = oldHome, oldCfg, oldDry, oldYes, oldProfile })

	configDir := filepath.Join(home, ".config", "ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, "skills", "mine"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"),
		[]byte("version = \"4.0\"\nname = \"me\"\npresets = [\"claude\", \"codex\"]\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "skills", "mine", "SKILL.md"),
		[]byte("---\nname: mine\ndescription: Does my thing. Use when I ask for my thing.\n---\nBody.\n"), 0o644))
	return home
}

func TestUserConfigPath(t *testing.T) {
	home, xdg, custom := t.TempDir(), t.TempDir(), t.TempDir()
	oldCfg := cfgFile
	t.Cleanup(func() { cfgFile = oldCfg })

	cfgFile = ""
	t.Setenv("XDG_CONFIG_HOME", "")
	assert.Equal(t, filepath.Join(home, ".config", "ai-rulez"), userConfigPath(home))

	t.Setenv("XDG_CONFIG_HOME", xdg)
	assert.Equal(t, filepath.Join(xdg, "ai-rulez"), userConfigPath(home))

	t.Setenv("XDG_CONFIG_HOME", "relative/ignored")
	assert.Equal(t, filepath.Join(home, ".config", "ai-rulez"), userConfigPath(home))

	cfgFile = custom
	assert.Equal(t, custom, userConfigPath(home))
}

func TestRunUserGenerate(t *testing.T) {
	t.Run("dry run lists targets and writes nothing", func(t *testing.T) {
		home := withUserHome(t)
		dryRun = true
		require.NoError(t, runUserGenerate(context.Background()))
		assert.NoDirExists(t, filepath.Join(home, ".claude"))
		assert.NoDirExists(t, filepath.Join(home, ".agents"))
	})

	t.Run("refuses to write without confirmation in a non-interactive shell", func(t *testing.T) {
		home := withUserHome(t)
		err := runUserGenerate(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nothing was written")
		assert.NoDirExists(t, filepath.Join(home, ".claude"))
	})

	t.Run("writes with --yes and records a manifest", func(t *testing.T) {
		home := withUserHome(t)
		assumeYes = true
		require.NoError(t, runUserGenerate(context.Background()))
		assert.FileExists(t, filepath.Join(home, ".claude", "skills", "mine", "SKILL.md"))
		assert.FileExists(t, filepath.Join(home, ".agents", "skills", "mine", "SKILL.md"))
		assert.FileExists(t, filepath.Join(home, ".config", "ai-rulez", ".generated-manifest.json"))

		cleanForce = true
		t.Cleanup(func() { cleanForce = false })
		require.NoError(t, runUserClean())
		assert.NoFileExists(t, filepath.Join(home, ".claude", "skills", "mine", "SKILL.md"))
		assert.NoFileExists(t, filepath.Join(home, ".config", "ai-rulez", ".generated-manifest.json"))
		assert.FileExists(t, filepath.Join(home, ".config", "ai-rulez", "config.toml"))
	})

	t.Run("a missing user config is reported with a hint", func(t *testing.T) {
		home := withUserHome(t)
		require.NoError(t, os.RemoveAll(filepath.Join(home, ".config", "ai-rulez")))
		err := runUserGenerate(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no user config found")
	})
}
