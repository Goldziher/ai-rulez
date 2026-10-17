package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkillInstall_EveryInstalledSkillFieldHasAFlag(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\n")
	t.Chdir(root)
	reset := func() {
		skillLocal, skillSource, skillPath, skillRef = false, "", "", ""
		skillInstallCmd.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Name == "source" {
				return
			}
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				_ = sv.Replace(nil)
			} else {
				_ = f.Value.Set(f.DefValue)
			}
			f.Changed = false
		})
	}
	reset()
	t.Cleanup(reset)
	flags := map[string]string{
		"source": "https://github.com/owner/repo", "version": "^1.2", "tag-prefix": "v", "include-prerelease": "true",
		"min-release-age": "7d", "domain": "backend", "profile": "dev", "include": "a*", "exclude": "*-wip",
		"name-prefix": "p-", "trust": "warn", "resources": "evals/**", "local-override": "vendor/pack",
	}
	for name, value := range flags {
		require.NoError(t, skillInstallCmd.Flags().Set(name, value), name)
	}

	// Act
	err := runSkillInstall(skillInstallCmd, []string{"pack"})

	// Assert
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(root, ".ai-rulez", "config.toml"))
	require.NoError(t, err)
	for _, want := range []string{
		"version = '^1.2'", "tag_prefix = 'v'", "include_prerelease = true", "min_release_age = '7d'",
		"domain = 'backend'", "'dev'", "'a*'", "'*-wip'", "name_prefix = 'p-'", "trust = 'warn'", "evals/**",
		"local_override = 'vendor/pack'",
	} {
		assert.Contains(t, string(data), want)
	}
}
