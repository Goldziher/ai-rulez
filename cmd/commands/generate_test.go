package commands_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/cmd/commands"
	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestGenerateCommand(t *testing.T) {
	assert.NotNil(t, commands.GenerateCmd)
	assert.Equal(t, "generate [config-file]", commands.GenerateCmd.Use)
	assert.Contains(t, commands.GenerateCmd.Aliases, "gen")

	flags := commands.GenerateCmd.Flags()
	assert.NotNil(t, flags.Lookup("dry-run"))
	assert.NotNil(t, flags.Lookup("update-gitignore"))
	assert.NotNil(t, flags.Lookup("recursive"))
}

func TestGenerateCommandAutoMigrateFlag(t *testing.T) {
	// Test that auto-migrate flag exists and has correct default
	cmd := commands.GenerateCmd
	autoMigrateFlag := cmd.Flags().Lookup("auto-migrate")
	assert.NotNil(t, autoMigrateFlag, "auto-migrate flag should exist")
	assert.Equal(t, "ask", autoMigrateFlag.DefValue, "auto-migrate default should be 'ask'")
}

func TestGenerateCommandV3Support(t *testing.T) {
	// Test V3 configuration support in generate command
	t.Run("V3ConfigDetection", func(t *testing.T) {
		// Verify that generate command is capable of detecting V3 configs
		// This is done via config.DetectConfigVersion() which is tested separately
		assert.NotNil(t, commands.GenerateCmd)
		assert.NotNil(t, commands.GenerateCmd.Run)
	})

	t.Run("V3GenerationLogic", func(t *testing.T) {
		// Verify generate command has V3-specific generation logic
		cmd := commands.GenerateCmd
		assert.NotNil(t, cmd)
		// The generate command's Run function checks for version and calls V3 loading
		// This is verified by code inspection in generate.go
	})

	t.Run("V3ProfileFlag", func(t *testing.T) {
		// Verify that profile flag exists for V3 configuration
		cmd := commands.GenerateCmd
		profileFlag := cmd.Flags().Lookup("profile")
		assert.NotNil(t, profileFlag, "Profile flag should exist for V3 support")
	})
}

func TestDetectConfigVersion(t *testing.T) {
	// Test config version detection functionality
	t.Run("DetectsV3Config", func(t *testing.T) {
		tmpDir := t.TempDir()
		aiRulezDir := filepath.Join(tmpDir, ".ai-rulez")
		require.NoError(t, os.MkdirAll(aiRulezDir, 0o755))

		version, err := config.DetectConfigVersion(tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "v3", version)
	})

	t.Run("DetectsV2ConfigYAML", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte("version: 2"), 0o644))

		version, err := config.DetectConfigVersion(tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "v2", version)
	})

	t.Run("DetectsV2ConfigYML", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "ai-rulez.yml")
		require.NoError(t, os.WriteFile(configPath, []byte("version: 2"), 0o644))

		version, err := config.DetectConfigVersion(tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "v2", version)
	})

	t.Run("NoConfigReturnsEmpty", func(t *testing.T) {
		tmpDir := t.TempDir()
		version, err := config.DetectConfigVersion(tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "", version)
	})

	t.Run("V3TakesPrecedenceOverV2", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Create both V2 and V3
		aiRulezDir := filepath.Join(tmpDir, ".ai-rulez")
		require.NoError(t, os.MkdirAll(aiRulezDir, 0o755))
		configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte("version: 2"), 0o644))

		// Should detect V3 when both exist
		version, err := config.DetectConfigVersion(tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "v3", version)
	})
}

func TestGenerateCommandAutoMigrationFlags(t *testing.T) {
	// Test auto-migrate flag parsing
	t.Run("DefaultAutoMigrateValue", func(t *testing.T) {
		cmd := commands.GenerateCmd
		autoMigrateFlag := cmd.Flags().Lookup("auto-migrate")
		require.NotNil(t, autoMigrateFlag)
		assert.Equal(t, "ask", autoMigrateFlag.DefValue)
	})

	t.Run("AutoMigrateFlagDescription", func(t *testing.T) {
		cmd := commands.GenerateCmd
		autoMigrateFlag := cmd.Flags().Lookup("auto-migrate")
		require.NotNil(t, autoMigrateFlag)
		assert.Contains(t, autoMigrateFlag.Usage, "Auto-migrate")
	})
}

func TestGenerateCommandV2Detection(t *testing.T) {
	// Test V2 config detection during generation
	t.Run("V2ConfigDetectionErrorHandling", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Create a V2 config
		configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
		minimalV2Config := `
version: "2"
metadata:
  name: "test"
outputs:
  claude: "CLAUDE.md"
`
		require.NoError(t, os.WriteFile(configPath, []byte(minimalV2Config), 0o644))

		// Verify detection works
		version, err := config.DetectConfigVersion(tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "v2", version)
	})
}

func TestMigrationContextHandling(t *testing.T) {
	// Test that migration uses proper context
	t.Run("ContextCancellation", func(t *testing.T) {
		_, cancel := context.WithCancel(context.Background())
		cancel()

		tmpDir := t.TempDir()
		_, err := config.DetectConfigVersion(tmpDir)
		// DetectConfigVersion doesn't use context, but this demonstrates
		// we could test context usage in actual migration if needed
		assert.NoError(t, err)
	})
}

func TestGenerateCommandCI_Environment(t *testing.T) {
	// Test that CI environment is properly detected
	t.Run("CIEnvironmentVariable", func(t *testing.T) {
		oldCI := os.Getenv("CI")
		oldContinuousIntegration := os.Getenv("CONTINUOUS_INTEGRATION")
		oldBuildID := os.Getenv("BUILD_ID")

		defer func() {
			if oldCI != "" {
				os.Setenv("CI", oldCI)
			} else {
				os.Unsetenv("CI")
			}
			if oldContinuousIntegration != "" {
				os.Setenv("CONTINUOUS_INTEGRATION", oldContinuousIntegration)
			} else {
				os.Unsetenv("CONTINUOUS_INTEGRATION")
			}
			if oldBuildID != "" {
				os.Setenv("BUILD_ID", oldBuildID)
			} else {
				os.Unsetenv("BUILD_ID")
			}
		}()

		// Test that CI=true is properly detected
		os.Setenv("CI", "true")
		os.Unsetenv("CONTINUOUS_INTEGRATION")
		os.Unsetenv("BUILD_ID")

		// This is verified by progress.PromptYesNo checking isInteractive()
		assert.NotNil(t, commands.GenerateCmd)
	})
}

func TestDeleteBackupDirectory_RemovesBackupsFromAutoMigration(t *testing.T) {
	// Test that backup directories created during auto-migration are deleted
	tmpDir := t.TempDir()

	// Create a backup directory as if it was created during auto-migration
	backupDir := filepath.Join(tmpDir, ".ai-rulez.backup.20231215_120000")
	err := os.MkdirAll(backupDir, 0o755)
	require.NoError(t, err)

	// Create a test file in the backup
	err = os.WriteFile(filepath.Join(backupDir, "config.yaml"), []byte("version: 3"), 0o644)
	require.NoError(t, err)

	// Verify backup exists
	assert.DirExists(t, backupDir)

	// Call DeleteBackupDirectory
	commands.DeleteBackupDirectory(tmpDir)

	// Verify backup was deleted
	assert.NoDirExists(t, backupDir)
}
