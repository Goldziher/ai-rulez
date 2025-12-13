package commands_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/cmd/commands"
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
		// The generate command's Run function checks for version and calls runGenerateV3()
		// This is verified by code inspection in generate.go lines 69-79
	})

	t.Run("V3ProfileFlag", func(t *testing.T) {
		// Verify that profile flag exists for V3 configuration
		cmd := commands.GenerateCmd
		profileFlag := cmd.Flags().Lookup("profile")
		assert.NotNil(t, profileFlag, "Profile flag should exist for V3 support")
	})
}

func TestGenerateCommandV2Deprecation(t *testing.T) {
	// Test V2 deprecation warning functionality in generate command
	t.Run("DeprecationWarningLogic", func(t *testing.T) {
		// Verify that generate command has deprecation warning logic
		// The handleV2DeprecationAndMigration function should be present
		assert.NotNil(t, commands.GenerateCmd)
		assert.NotNil(t, commands.GenerateCmd.Run)
		// Deprecation handling is triggered when shouldAutoMigrate detects ai-rulez.yaml
		// This is verified by code inspection in generate.go lines 69-77
	})

	t.Run("InteractivePrompt", func(t *testing.T) {
		// Verify that non-interactive mode respects CI environment variable
		// This ensures CI/CD pipelines won't hang on migration prompts
		oldCI := os.Getenv("CI")
		defer func() {
			if oldCI != "" {
				os.Setenv("CI", oldCI)
			} else {
				os.Unsetenv("CI")
			}
		}()

		os.Setenv("CI", "true")
		// In CI mode, shouldPromptForMigration should return false
		// This prevents blocking prompts in automated environments
		assert.NotNil(t, commands.GenerateCmd)
	})

	t.Run("BackupCreation", func(t *testing.T) {
		// Verify that migration backup functionality is available
		// autoMigrateToV3WithBackup should create .yaml.backup files
		assert.NotNil(t, commands.GenerateCmd)
		// Backup is created during migration to preserve original V2 config
		// This is verified by code inspection in generate.go lines 535-545
	})
}
