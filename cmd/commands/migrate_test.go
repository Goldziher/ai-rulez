package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// migrate has a real subcommand per target, so completion offers them and
// `migrate banana` is an unknown command rather than a runtime error.
func TestMigrateCommand(t *testing.T) {
	assert.Equal(t, "migrate", MigrateCmd.Use)
	assert.True(t, MigrateCmd.HasSubCommands(), "migrate alone is a group: it prints help")
	var names []string
	for _, c := range MigrateCmd.Commands() {
		names = append(names, c.Name())
	}
	assert.ElementsMatch(t, []string{"v5", "okf"}, names)
}

func TestMigrateCommand_Flags(t *testing.T) {
	for _, name := range []string{"dry-run", "check", "format"} {
		assert.NotNil(t, MigrateCmd.PersistentFlags().Lookup(name), name)
	}
	v5, _, err := MigrateCmd.Find([]string{"v5"})
	require.NoError(t, err)
	okf, _, err := MigrateCmd.Find([]string{"okf"})
	require.NoError(t, err)
	for _, name := range []string{"adopt-defaults", "write", "recursive"} {
		assert.NotNil(t, v5.Flags().Lookup(name), name)
		assert.Nil(t, okf.Flags().Lookup(name), "%s is meaningless for okf", name)
	}
	assert.Nil(t, v5.LocalNonPersistentFlags().Lookup("config-dir"), "--config-dir is the global flag")
}

func TestMigrateCommand_ExitCodes(t *testing.T) {
	project := t.TempDir()
	dir := filepath.Join(project, ".ai-rulez")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte("version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n"), 0o644))
	t.Chdir(project)

	require.NoError(t, MigrateCmd.PersistentFlags().Set("check", "true"))
	t.Cleanup(func() { _ = MigrateCmd.PersistentFlags().Set("check", "false") })
	assert.Equal(t, 2, runMigrateV5(os.Stderr), "pending migration under --check is drift")

	require.NoError(t, MigrateCmd.PersistentFlags().Set("check", "false"))
	assert.Equal(t, 0, runMigrateV5(os.Stderr))
	assert.Equal(t, 0, runMigrateV5(os.Stderr), "second run has nothing to do")
}

func TestMigrateV5TargetsExplicitChildConfiguration(t *testing.T) {
	for _, locator := range []string{
		".ai-rulez", ".ai-rulez/config.toml", ".config/ai-rulez", ".config/ai-rulez/config.yaml",
	} {
		t.Run(locator, func(t *testing.T) {
			parent := t.TempDir()
			writeFile(t, filepath.Join(parent, ".ai-rulez/config.toml"), "version = \"5.0\"\nname = \"parent\"\n")
			child := filepath.Join(parent, "plugin")
			target := filepath.Join(child, locator)
			dir := target
			body := "version = \"4.0\"\nname = \"child\"\npresets = [\"claude\"]\n"
			if filepath.Ext(target) != "" && filepath.Base(target) != ".ai-rulez" {
				dir = filepath.Dir(target)
			} else {
				target = filepath.Join(target, "config.toml")
			}
			if filepath.Ext(target) == ".yaml" {
				body = "version: '4.0'\nname: child\npresets: [claude]\n"
			}
			writeFile(t, target, body)
			t.Chdir(parent)
			oldConfig, oldDir, oldCheck := cfgFile, configDir, migrateCheck
			oldFormat := migrateFormat
			t.Cleanup(func() { cfgFile, configDir, migrateCheck, migrateFormat = oldConfig, oldDir, oldCheck, oldFormat })
			require.NoError(t, RootCmd.PersistentFlags().Set("config", filepath.Join(child, locator)))
			configDir, migrateCheck, migrateFormat = "", true, formatText
			var output bytes.Buffer
			assert.Equal(t, exitDrift, runMigrateV5(&output))
			assert.Contains(t, output.String(), "would migrate 1, unchanged 0, errors 0")
			got, err := os.ReadFile(target)
			require.NoError(t, err)
			assert.Equal(t, body, string(got), "--check must leave the selected config unchanged")
			migrateCheck = false
			assert.Equal(t, 0, runMigrateV5(&output))
			got, err = os.ReadFile(filepath.Join(dir, "config.toml"))
			require.NoError(t, err)
			assert.Contains(t, string(got), `version = "5.0"`)
			got, err = os.ReadFile(filepath.Join(parent, ".ai-rulez/config.toml"))
			require.NoError(t, err)
			assert.Equal(t, "version = \"5.0\"\nname = \"parent\"\n", string(got))
		})
	}
}
