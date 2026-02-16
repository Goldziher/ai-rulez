package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestInitCommandE2E(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		args        []string
		setupFiles  func(t *testing.T, dir string)
		validate    func(t *testing.T, configPath string)
	}{
		{
			name:        "Basic init with project name",
			projectName: "TestProject",
			args:        []string{},
			setupFiles:  func(t *testing.T, dir string) {},
			validate: func(t *testing.T, configPath string) {
				content, err := os.ReadFile(configPath)
				require.NoError(t, err)

				// V3 config should have version, name, and presets
				assert.Contains(t, string(content), "version:")
				assert.Contains(t, string(content), "TestProject")

				var config map[string]interface{}
				err = yaml.Unmarshal(content, &config)
				assert.NoError(t, err, "Generated YAML should be valid")
			},
		},
		{
			name:        "Init with skip-content flag",
			projectName: "GoProject",
			args:        []string{"--skip-content"},
			setupFiles: func(t *testing.T, dir string) {
				goModContent := `module example.com/goproject
go 1.21

require github.com/stretchr/testify v1.8.4`
				err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0o644)
				require.NoError(t, err)
			},
			validate: func(t *testing.T, configPath string) {
				content, err := os.ReadFile(configPath)
				require.NoError(t, err)

				var config map[string]interface{}
				err = yaml.Unmarshal(content, &config)
				assert.NoError(t, err, "Config should be valid YAML")
			},
		},
		{
			name:        "Init with skip-mcp flag",
			projectName: "TSProject",
			args:        []string{"--skip-mcp"},
			setupFiles: func(t *testing.T, dir string) {
				packageJSON := `{
  "name": "ts-project",
  "version": "1.0.0",
  "scripts": {
    "build": "tsc",
    "test": "jest",
    "lint": "eslint ."
  },
  "devDependencies": {
    "typescript": "^5.0.0",
    "@types/node": "^20.0.0"
  },
  "dependencies": {
    "express": "^4.18.0"
  }
}`
				err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(packageJSON), 0o644)
				require.NoError(t, err)
			},
			validate: func(t *testing.T, configPath string) {
				content, err := os.ReadFile(configPath)
				require.NoError(t, err)

				var config map[string]interface{}
				err = yaml.Unmarshal(content, &config)
				assert.NoError(t, err)
			},
		},
		{
			name:        "Init with domains flag",
			projectName: "MultiDomain",
			args:        []string{"--domains", "backend,frontend,infra"},
			setupFiles:  func(t *testing.T, dir string) {},
			validate: func(t *testing.T, configPath string) {
				content, err := os.ReadFile(configPath)
				require.NoError(t, err)

				var config map[string]interface{}
				err = yaml.Unmarshal(content, &config)
				assert.NoError(t, err, "Config should be valid YAML")

				// Check that domains are created under .ai-rulez/domains/
				// configPath is at .ai-rulez/config.yaml
				aiRulesDir := filepath.Dir(configPath)
				domainsDir := filepath.Join(aiRulesDir, "domains")
				for _, domain := range []string{"backend", "frontend", "infra"} {
					domainDir := filepath.Join(domainsDir, domain)
					assert.DirExists(t, domainDir, "Domain directory should exist: %s", domain)
				}
			},
		},
		{
			name:        "Init with json format",
			projectName: "JSONProject",
			args:        []string{"--format", "json"},
			setupFiles:  func(t *testing.T, dir string) {},
			validate: func(t *testing.T, configPath string) {
				// For JSON format, config might be in a different format
				// Just verify it's valid and can be read
				content, err := os.ReadFile(configPath)
				require.NoError(t, err)
				assert.True(t, len(content) > 0, "Config file should have content")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			oldDir, err := os.Getwd()
			require.NoError(t, err)
			err = os.Chdir(dir)
			require.NoError(t, err)
			defer os.Chdir(oldDir)

			tt.setupFiles(t, dir)

			args := []string{"init", tt.projectName}
			args = append(args, tt.args...)

			output, err := runTestCommand(args...)
			require.NoError(t, err, "Init command should succeed. Output: %s", output)

			configPath := filepath.Join(dir, ".ai-rulez", "config.yaml")
			if strings.Contains(strings.Join(tt.args, " "), "--format json") {
				configPath = filepath.Join(dir, ".ai-rulez", "config.json")
			}
			assert.FileExists(t, configPath, "config file should be created at %s", configPath)

			if tt.validate != nil {
				tt.validate(t, configPath)
			}

			output, err = runTestCommand("validate")
			assert.NoError(t, err, "Generated config should be valid. Output: %s", output)
		})
	}
}

func runTestCommand(args ...string) (string, error) {
	binaryPath := "/tmp/ai-rulez-test-binary"
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd")
	// Use current working directory instead of hardcoded path
	// This allows tests to run on any developer's machine
	buildCmd.Env = append(os.Environ(), "NO_INTERACTIVE=1")
	if err := buildCmd.Run(); err != nil {
		return "", err
	}

	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(), "NO_INTERACTIVE=1")
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func TestInitCommandWithAgentGeneration(t *testing.T) {
	t.Skip("Skipping agent generation test - requires actual AI agent CLIs")

	tests := []struct {
		name         string
		agent        string
		mockResponse string
		validate     func(t *testing.T, content string)
	}{
		{
			name:  "Claude agent generation",
			agent: "claude",
			mockResponse: `# AI-Rulez Configuration v2.0
$schema: https://github.com/Goldziher/ai-rulez/schema/ai-rules-v2.schema.json

metadata:
  name: "TestProject"
  description: "AI-assisted project"

outputs:
  - path: "CLAUDE.md"

agents:
  - name: "architect"
    description: "System design specialist"
    priority: high
    system_prompt: |
      You are a system architect.
      
rules:
  - name: "Code Quality"
    priority: high
    content: |
      Maintain high code quality standards.`,
			validate: func(t *testing.T, content string) {
				assert.Contains(t, content, "agents:")
				assert.NotContains(t, content, "# agents:")
				assert.Contains(t, content, "rules:")
				assert.NotContains(t, content, "# rules:")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
		})
	}
}
