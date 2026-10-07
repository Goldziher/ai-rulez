package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The import gate (skill signatures, then the scan of imported content) guards
// every write of generated output: the in-repo run, a plugin bundle and the user
// scope. Only a dry run, which writes nothing, skips it.
func TestImportGateRunsOnEveryWritingMode(t *testing.T) {
	tests := []struct {
		name    string
		plugin  bool
		dry     bool
		wantErr bool
	}{
		{"plain generate", false, false, true},
		{"plugin bundle", true, false, true},
		{"dry run", false, true, false},
		{"plugin dry run", true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := importingConfig(t, "error")
			oldPlugin, oldDry := pluginMode, dryRun
			pluginMode, dryRun = tt.plugin, tt.dry
			t.Cleanup(func() { pluginMode, dryRun = oldPlugin, oldDry })

			// Act
			err := importGate(cfg)

			// Assert
			assert.Equal(t, tt.wantErr, err != nil)
		})
	}
}

func TestRunUserGenerateRefusesImportedSecrets(t *testing.T) {
	// Arrange: the user config installs a skill that carries a secret
	home := withUserHome(t)
	assumeYes = true
	vendor := filepath.Join(t.TempDir(), "vendor", "imp")
	require.NoError(t, os.MkdirAll(vendor, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(vendor, "SKILL.md"),
		[]byte("---\nname: imp\ndescription: Use when testing the imported content scan.\n---\nkey AKIAIOSFODNN7EXAMPLE\n"), 0o644))
	configPath := filepath.Join(home, ".config", "ai-rulez", "config.toml")
	require.NoError(t, os.WriteFile(configPath,
		[]byte("version = \"5.0\"\nname = \"me\"\npresets = [\"claude\"]\n\n[[installed_skills]]\nname = \"imp\"\nsource = \""+filepath.ToSlash(vendor)+"\"\npath = \".\"\n"), 0o644))

	// Act
	err := runUserGenerate(context.Background())

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "security scan")
	assert.NoDirExists(t, filepath.Join(home, ".claude"), "nothing was written")
}

func TestImportGateRefusesASecretInAuthoredContent(t *testing.T) {
	tests := []struct {
		name    string
		dry     bool
		body    string
		wantErr bool
	}{
		{"secret in a rule", false, "# Style\nkey " + "AKIA" + "IOSFODNN7EXAMPLE\n", true},
		{"secret in a rule, dry run", true, "# Style\nkey " + "AKIA" + "IOSFODNN7EXAMPLE\n", false},
		{"clean rule", false, "# Style\nUse tabs.\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := lockProject(t, "")
			writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), tt.body)
			cfg, err := loadForLock("")
			require.NoError(t, err)
			oldDry := dryRun
			dryRun = tt.dry
			t.Cleanup(func() { dryRun = oldDry })

			// Act
			err = importGate(cfg)

			// Assert
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "AR001")
		})
	}
}
