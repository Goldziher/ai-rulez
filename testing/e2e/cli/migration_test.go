package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/testing/e2e/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestV1ToV2Migration(t *testing.T) {
	t.Run("automatic_migration_on_generate", func(t *testing.T) {
		tmpDir := t.TempDir()

		v1Config := `metadata:
  name: TestMigration
  description: Testing v1 to v2 migration

outputs:
  - path: .cursorrules
    template: default
  - path: docs.md
    template: "@templates/doc.tmpl"
  - path: inline.md
    template: |
      # Rules
      {{range .Rules}}
      - {{.Content}}
      {{end}}

rules:
  - name: test-rule
    content: Always use TypeScript
    priority: high

agents:
  - name: code-review
    description: Code review specialist
    template: agent
`
		configPath := filepath.Join(tmpDir, "ai_rulez.yaml")
		err := os.WriteFile(configPath, []byte(v1Config), 0o644)
		require.NoError(t, err)

		templatesDir := filepath.Join(tmpDir, "templates")
		err = os.MkdirAll(templatesDir, 0o755)
		require.NoError(t, err)

		templateContent := `# Documentation
{{range .Rules}}
- {{.Name}}: {{.Content}}
{{end}}`
		err = os.WriteFile(filepath.Join(templatesDir, "doc.tmpl"), []byte(templateContent), 0o644)
		require.NoError(t, err)

		result := testutil.RunCLIExpectSuccess(t, tmpDir, "generate")

		assert.Contains(t, result.Stdout, "Successfully migrated configuration from v1 to v2")

		assert.Contains(t, result.Stdout, "Generated 3 file(s) successfully")

		migratedData, err := os.ReadFile(configPath)
		require.NoError(t, err)

		var migratedConfig map[string]interface{}
		err = yaml.Unmarshal(migratedData, &migratedConfig)
		require.NoError(t, err)

		outputs := migratedConfig["outputs"].([]interface{})

		output1 := outputs[0].(map[string]interface{})
		template1 := output1["template"].(map[string]interface{})
		assert.Equal(t, "builtin", template1["type"])
		assert.Equal(t, "default", template1["value"])

		output2 := outputs[1].(map[string]interface{})
		template2 := output2["template"].(map[string]interface{})
		assert.Equal(t, "file", template2["type"])
		assert.Equal(t, "templates/doc.tmpl", template2["value"])

		output3 := outputs[2].(map[string]interface{})
		template3 := output3["template"].(map[string]interface{})
		assert.Equal(t, "inline", template3["type"])
		assert.Contains(t, template3["value"], "# Rules")

		agents := migratedConfig["agents"].([]interface{})
		agent := agents[0].(map[string]interface{})
		agentTemplate := agent["template"].(map[string]interface{})
		assert.Equal(t, "builtin", agentTemplate["type"])
		assert.Equal(t, "agent", agentTemplate["value"])

		assert.FileExists(t, filepath.Join(tmpDir, ".cursorrules"))
		assert.FileExists(t, filepath.Join(tmpDir, "docs.md"))
		assert.FileExists(t, filepath.Join(tmpDir, "inline.md"))
	})

	t.Run("migration_fails_with_includes", func(t *testing.T) {
		tmpDir := t.TempDir()

		v1ConfigWithIncludes := `metadata:
  name: TestMigrationFail

includes:
  - ./shared.yaml

outputs:
  - path: .cursorrules
    template: default
`
		configPath := filepath.Join(tmpDir, "ai_rulez.yaml")
		err := os.WriteFile(configPath, []byte(v1ConfigWithIncludes), 0o644)
		require.NoError(t, err)

		result := testutil.RunCLIExpectError(t, tmpDir, "generate")

		assert.Contains(t, result.Stderr, "cannot automatically migrate configuration with 'includes'")
		assert.Contains(t, result.Stderr, "Remove the includes and merge the configuration manually")
	})

	t.Run("no_migration_for_v2_config", func(t *testing.T) {
		tmpDir := t.TempDir()

		v2Config := `metadata:
  name: TestV2Config

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
		configPath := filepath.Join(tmpDir, "ai_rulez.yaml")
		err := os.WriteFile(configPath, []byte(v2Config), 0o644)
		require.NoError(t, err)

		result := testutil.RunCLIExpectSuccess(t, tmpDir, "generate")

		assert.NotContains(t, result.Stdout, "migrated")

		assert.Contains(t, result.Stdout, "Generated 1 file(s) successfully")

		afterData, err := os.ReadFile(configPath)
		require.NoError(t, err)
		assert.Equal(t, v2Config, string(afterData))
	})

	t.Run("migration_with_various_template_types", func(t *testing.T) {
		tmpDir := t.TempDir()

		v1Config := `metadata:
  name: TemplateVariety

outputs:
  - path: inline1.md
    template: "Simple text"
  - path: inline2.md
    template: |
      Multi
      Line
      Template
`
		configPath := filepath.Join(tmpDir, "ai_rulez.yaml")
		err := os.WriteFile(configPath, []byte(v1Config), 0o644)
		require.NoError(t, err)

		result := testutil.RunCLI(t, tmpDir, "generate", "--dry-run")

		assert.Equal(t, 0, result.ExitCode)

		migratedData, err := os.ReadFile(configPath)
		require.NoError(t, err)

		var config map[string]interface{}
		err = yaml.Unmarshal(migratedData, &config)
		require.NoError(t, err)

		outputs := config["outputs"].([]interface{})

		checkTemplate(t, outputs[0], "inline", "Simple text")

		output1 := outputs[1].(map[string]interface{})
		template1 := output1["template"].(map[string]interface{})
		assert.Equal(t, "inline", template1["type"])
		assert.Contains(t, template1["value"], "Multi")
	})

	t.Run("recursive_migration", func(t *testing.T) {
		tmpDir := t.TempDir()

		subdir1 := filepath.Join(tmpDir, "project1")
		subdir2 := filepath.Join(tmpDir, "project2")

		err := os.MkdirAll(subdir1, 0o755)
		require.NoError(t, err)
		err = os.MkdirAll(subdir2, 0o755)
		require.NoError(t, err)

		v1Config1 := `metadata:
  name: Project1
outputs:
  - path: .cursorrules
    template: default`

		v1Config2 := `metadata:
  name: Project2
outputs:
  - path: .cursorrules
    template: documentation`

		err = os.WriteFile(filepath.Join(subdir1, "ai_rulez.yaml"), []byte(v1Config1), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(subdir2, "ai_rulez.yaml"), []byte(v1Config2), 0o644)
		require.NoError(t, err)

		result := testutil.RunCLIExpectSuccess(t, tmpDir, "generate", "--recursive", "--dry-run")

		assert.Contains(t, result.Stdout, "Found 2 configuration file(s)")

		config1Data, _ := os.ReadFile(filepath.Join(subdir1, "ai_rulez.yaml"))
		assert.Contains(t, string(config1Data), "type: builtin")

		config2Data, _ := os.ReadFile(filepath.Join(subdir2, "ai_rulez.yaml"))
		assert.Contains(t, string(config2Data), "type: builtin")
	})
}

func checkTemplate(t *testing.T, output interface{}, expectedType, expectedValue string) {
	t.Helper()
	outputMap := output.(map[string]interface{})
	template := outputMap["template"].(map[string]interface{})
	assert.Equal(t, expectedType, template["type"])
	assert.Equal(t, expectedValue, template["value"])
}

func TestMigrationBackup(t *testing.T) {
	tmpDir := t.TempDir()

	v1Config := `metadata:
  name: BackupTest
outputs:
  - path: .cursorrules
    template: default`

	configPath := filepath.Join(tmpDir, "ai_rulez.yaml")
	originalContent := []byte(v1Config)
	err := os.WriteFile(configPath, originalContent, 0o644)
	require.NoError(t, err)

	testutil.RunCLIExpectSuccess(t, tmpDir, "generate")

	migratedData, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.NotEqual(t, string(originalContent), string(migratedData))
	assert.Contains(t, string(migratedData), "type: builtin")
}

func TestMigrationErrorHandling(t *testing.T) {
	t.Run("invalid_yaml", func(t *testing.T) {
		tmpDir := t.TempDir()

		invalidYAML := `metadata:
  name: Invalid
outputs: [
  this is not valid yaml
`
		configPath := filepath.Join(tmpDir, "ai_rulez.yaml")
		err := os.WriteFile(configPath, []byte(invalidYAML), 0o644)
		require.NoError(t, err)

		result := testutil.RunCLIExpectError(t, tmpDir, "generate")
		assert.Contains(t, strings.ToLower(result.Stderr), "error")
	})
}
