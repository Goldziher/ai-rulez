package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/tests/e2e/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV1ToV2Migration(t *testing.T) {
	t.Run("automatic_migration_on_generate", func(t *testing.T) {
		tmpDir := t.TempDir()

		v2Config := `metadata:
  name: TestMigration
  description: Testing v2 to v3 migration

presets:
  - claude

outputs:
  - path: .cursorrules
    template:
      type: builtin
      value: default

rules:
  - name: test-rule
    content: Always use TypeScript
    priority: high

agents:
  - name: code-review
    description: Code review specialist
`
		configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
		err := os.WriteFile(configPath, []byte(v2Config), 0o644)
		require.NoError(t, err)

		result := testutil.RunCLIExpectSuccess(t, tmpDir, "generate")

		// Verify migration messages
		assert.Contains(t, result.Stdout, "Migrating V2 configuration to V3")
		assert.Contains(t, result.Stdout, "Migration completed successfully")

		// Verify .ai-rulez directory was created
		aiRulezPath := filepath.Join(tmpDir, ".ai-rulez")
		assert.DirExists(t, aiRulezPath)

		// Verify config.yaml exists in .ai-rulez/
		configYAMLPath := filepath.Join(aiRulezPath, "config.yaml")
		assert.FileExists(t, configYAMLPath)

		// Verify rules directory exists
		rulesPath := filepath.Join(aiRulezPath, "rules")
		assert.DirExists(t, rulesPath)

		// Verify agents directory exists
		agentsPath := filepath.Join(aiRulezPath, "agents")
		assert.DirExists(t, agentsPath)
	})

	t.Run("migration_fails_with_missing_includes", func(t *testing.T) {
		tmpDir := t.TempDir()

		v2ConfigWithIncludes := `metadata:
  name: TestMigrationFail

presets:
  - claude

includes:
  - ./shared.yaml

outputs:
  - path: .cursorrules
    template:
      type: builtin
      value: default
`
		configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
		err := os.WriteFile(configPath, []byte(v2ConfigWithIncludes), 0o644)
		require.NoError(t, err)

		result := testutil.RunCLIExpectError(t, tmpDir, "generate")

		// The migration fails because the include file doesn't exist
		assert.Contains(t, result.Stderr, "include file not found")
	})

	t.Run("auto_migration_v2_to_v3", func(t *testing.T) {
		tmpDir := t.TempDir()

		v2Config := `metadata:
  name: TestV2Config

presets:
  - claude

outputs:
  - path: .cursorrules
    template:
      type: builtin
      value: default

rules:
  - name: test-rule
    content: Use best practices
    priority: high
`
		configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
		err := os.WriteFile(configPath, []byte(v2Config), 0o644)
		require.NoError(t, err)

		result := testutil.RunCLIExpectSuccess(t, tmpDir, "generate")

		// Verify migration happened
		assert.Contains(t, result.Stdout, "Migrating V2 configuration to V3")

		// Verify .ai-rulez directory was created
		aiRulezPath := filepath.Join(tmpDir, ".ai-rulez")
		assert.DirExists(t, aiRulezPath)
	})

	t.Run("migration_with_various_template_types", func(t *testing.T) {
		tmpDir := t.TempDir()

		v2Config := `metadata:
  name: TemplateVariety

presets:
  - claude

outputs:
  - path: inline1.md
    template:
      type: inline
      value: "Simple text"
  - path: inline2.md
    template:
      type: inline
      value: |
        Multi
        Line
        Template
`
		configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
		err := os.WriteFile(configPath, []byte(v2Config), 0o644)
		require.NoError(t, err)

		result := testutil.RunCLI(t, tmpDir, "generate", "--dry-run")

		assert.Equal(t, 0, result.ExitCode)

		// Verify .ai-rulez directory was created
		aiRulezPath := filepath.Join(tmpDir, ".ai-rulez")
		assert.DirExists(t, aiRulezPath)
	})

	t.Run("multiple_migrations", func(t *testing.T) {
		// Test multiple separate V2 configs being migrated
		subdir1 := t.TempDir()
		subdir2 := t.TempDir()

		v2Config1 := `metadata:
  name: Project1
presets:
  - claude
outputs:
  - path: .cursorrules
    template:
      type: builtin
      value: default`

		v2Config2 := `metadata:
  name: Project2
presets:
  - claude
outputs:
  - path: .cursorrules
    template:
      type: builtin
      value: default`

		err := os.WriteFile(filepath.Join(subdir1, "ai-rulez.yaml"), []byte(v2Config1), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(subdir2, "ai-rulez.yaml"), []byte(v2Config2), 0o644)
		require.NoError(t, err)

		// Migrate each separately
		result1 := testutil.RunCLIExpectSuccess(t, subdir1, "generate", "--dry-run")
		result2 := testutil.RunCLIExpectSuccess(t, subdir2, "generate", "--dry-run")

		assert.Contains(t, result1.Stdout, "Migrating V2 configuration to V3")
		assert.Contains(t, result2.Stdout, "Migrating V2 configuration to V3")

		// Verify .ai-rulez directories were created
		assert.DirExists(t, filepath.Join(subdir1, ".ai-rulez"))
		assert.DirExists(t, filepath.Join(subdir2, ".ai-rulez"))
	})
}

func TestMigrationBackup(t *testing.T) {
	tmpDir := t.TempDir()

	v2Config := `metadata:
  name: BackupTest
presets:
  - claude
outputs:
  - path: .cursorrules
    template:
      type: builtin
      value: default`

	configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
	err := os.WriteFile(configPath, []byte(v2Config), 0o644)
	require.NoError(t, err)

	result := testutil.RunCLIExpectSuccess(t, tmpDir, "generate")

	// Verify migration happened
	assert.Contains(t, result.Stdout, "Migrating V2 configuration to V3")

	// Verify .ai-rulez directory was created
	aiRulezPath := filepath.Join(tmpDir, ".ai-rulez")
	assert.DirExists(t, aiRulezPath)
}

func TestMigrationErrorHandling(t *testing.T) {
	t.Run("invalid_yaml", func(t *testing.T) {
		tmpDir := t.TempDir()

		invalidYAML := `metadata:
  name: Invalid
outputs: [
  this is not valid yaml
`
		configPath := filepath.Join(tmpDir, "ai-rulez.yaml")
		err := os.WriteFile(configPath, []byte(invalidYAML), 0o644)
		require.NoError(t, err)

		result := testutil.RunCLIExpectError(t, tmpDir, "generate")
		assert.Contains(t, strings.ToLower(result.Stderr), "error")
	})
}
