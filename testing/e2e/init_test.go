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
		providers   []string
		setupFiles  func(t *testing.T, dir string)
		validate    func(t *testing.T, configPath string)
	}{
		{
			name:        "Basic init with claude provider",
			projectName: "TestProject",
			providers:   []string{"claude"},
			setupFiles:  func(t *testing.T, dir string) {},
			validate: func(t *testing.T, configPath string) {
				content, err := os.ReadFile(configPath)
				require.NoError(t, err)

				assert.Contains(t, string(content), "metadata:")
				assert.Contains(t, string(content), `name: "TestProject"`)
				assert.Contains(t, string(content), "outputs:")
				assert.Contains(t, string(content), `path: "CLAUDE.md"`)

				assert.Contains(t, string(content), "# agents:")
				assert.Contains(t, string(content), "# rules:")
				assert.Contains(t, string(content), "# sections:")
				assert.Contains(t, string(content), "# commands:")
				assert.Contains(t, string(content), "# mcp_servers:")

				assert.Contains(t, string(content), "ai-rules-v2.schema.json")

				var config map[string]interface{}
				err = yaml.Unmarshal(content, &config)
				assert.NoError(t, err, "Generated YAML should be valid")
			},
		},
		{
			name:        "Init with Go project",
			projectName: "GoProject",
			providers:   []string{"cursor"},
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

				assert.Contains(t, string(content), `path: ".cursorrules"`)

				contentStr := string(content)
				assert.True(t, strings.Contains(contentStr, "# agents:") ||
					strings.Contains(contentStr, "#   - name:"))
			},
		},
		{
			name:        "Init with TypeScript project",
			projectName: "TSProject",
			providers:   []string{"gemini"},
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

				assert.Contains(t, string(content), `path: "GEMINI.md"`)

				var config map[string]interface{}
				err = yaml.Unmarshal(content, &config)
				assert.NoError(t, err)
			},
		},
		{
			name:        "Init with multiple providers",
			projectName: "MultiProvider",
			providers:   []string{"claude", "cursor", "gemini"},
			setupFiles:  func(t *testing.T, dir string) {},
			validate: func(t *testing.T, configPath string) {
				content, err := os.ReadFile(configPath)
				require.NoError(t, err)

				assert.Contains(t, string(content), `path: "CLAUDE.md"`)
				assert.Contains(t, string(content), `path: ".cursor/rules/"`)
				assert.Contains(t, string(content), `path: "GEMINI.md"`)
			},
		},
		{
			name:        "Init with monorepo",
			projectName: "MonorepoProject",
			providers:   []string{"amp"},
			setupFiles: func(t *testing.T, dir string) {
				lernaContent := `{
  "packages": ["packages/*"],
  "version": "independent"
}`
				err := os.WriteFile(filepath.Join(dir, "lerna.json"), []byte(lernaContent), 0o644)
				require.NoError(t, err)

				packageJSON := `{
  "name": "monorepo-root",
  "private": true,
  "workspaces": ["packages/*"]
}`
				err = os.WriteFile(filepath.Join(dir, "package.json"), []byte(packageJSON), 0o644)
				require.NoError(t, err)
			},
			validate: func(t *testing.T, configPath string) {
				content, err := os.ReadFile(configPath)
				require.NoError(t, err)

				assert.Contains(t, string(content), `path: "AGENTS.md"`)

				var config map[string]interface{}
				err = yaml.Unmarshal(content, &config)
				assert.NoError(t, err)
			},
		},
		{
			name:        "Init with Taskfile",
			projectName: "TaskfileProject",
			providers:   []string{"claude"},
			setupFiles: func(t *testing.T, dir string) {
				goModContent := `module example.com/taskproject
go 1.21`
				err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0o644)
				require.NoError(t, err)

				taskfileContent := `version: '3'

tasks:
  build:
    cmds:
      - go build ./...
  test:
    cmds:
      - go test ./...
  lint:
    cmds:
      - golangci-lint run`
				err = os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(taskfileContent), 0o644)
				require.NoError(t, err)
			},
			validate: func(t *testing.T, configPath string) {
				content, err := os.ReadFile(configPath)
				require.NoError(t, err)

				assert.Contains(t, string(content), "# commands:")

				var config map[string]interface{}
				err = yaml.Unmarshal(content, &config)
				assert.NoError(t, err)
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
			for _, provider := range tt.providers {
				args = append(args, "--"+provider)
			}

			output, err := runTestCommand(args...)
			require.NoError(t, err, "Init command should succeed. Output: %s", output)

			configPath := filepath.Join(dir, "ai_rulez.yaml")
			assert.FileExists(t, configPath, "ai_rulez.yaml should be created")

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
	buildCmd.Dir = "/Users/naamanhirschfeld/workspace/ai_rulez"
	if err := buildCmd.Run(); err != nil {
		return "", err
	}

	cmd := exec.Command(binaryPath, args...)
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
