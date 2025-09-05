package agents

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyzeCodebase(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T, dir string)
		expected func(info CodebaseInfo) bool
	}{
		{
			name: "Go project with go.mod",
			setup: func(t *testing.T, dir string) {
				goModContent := `module example.com/testproject

go 1.21

require (
	github.com/stretchr/testify v1.8.4
)`
				err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0o644)
				require.NoError(t, err)
			},
			expected: func(info CodebaseInfo) bool {
				return info.MainLanguage == "Go" &&
					contains(info.TechStack, "Go") &&
					contains(info.ConfigFiles, "go.mod") &&
					info.BuildCommand == "go build" &&
					info.TestCommand == "go test ./..." &&
					info.LintCommand == "golangci-lint run"
			},
		},
		{
			name: "TypeScript project with package.json",
			setup: func(t *testing.T, dir string) {
				packageJSON := `{
  "name": "test-project",
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
    "react": "^18.0.0"
  }
}`
				err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(packageJSON), 0o644)
				require.NoError(t, err)
			},
			expected: func(info CodebaseInfo) bool {
				return info.MainLanguage == "TypeScript" &&
					contains(info.TechStack, "TypeScript") &&
					contains(info.TechStack, "React") &&
					contains(info.ConfigFiles, "package.json") &&
					info.BuildCommand == "npm run build" &&
					info.TestCommand == "npm test" &&
					info.LintCommand == "npm run lint"
			},
		},
		{
			name: "Python project with pyproject.toml",
			setup: func(t *testing.T, dir string) {
				pyprojectContent := `[tool.poetry]
name = "test-project"
version = "0.1.0"
description = ""

[tool.poetry.dependencies]
python = "^3.11"
django = "^4.2"

[build-system]
requires = ["poetry-core"]
build-backend = "poetry.core.masonry.api"
`
				err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(pyprojectContent), 0o644)
				require.NoError(t, err)
			},
			expected: func(info CodebaseInfo) bool {
				return info.MainLanguage == "Python" &&
					contains(info.TechStack, "Python") &&
					contains(info.ConfigFiles, "pyproject.toml") &&
					info.BuildCommand == "poetry build" &&
					info.TestCommand == "poetry run pytest" &&
					info.LintCommand == "poetry run flake8"
			},
		},
		{
			name: "Rust project with Cargo.toml",
			setup: func(t *testing.T, dir string) {
				cargoContent := `[package]
name = "test-project"
version = "0.1.0"
edition = "2021"

[dependencies]
tokio = { version = "1", features = ["full"] }
`
				err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(cargoContent), 0o644)
				require.NoError(t, err)
			},
			expected: func(info CodebaseInfo) bool {
				return info.MainLanguage == "Rust" &&
					contains(info.TechStack, "Rust") &&
					contains(info.ConfigFiles, "Cargo.toml") &&
					info.BuildCommand == "cargo build" &&
					info.TestCommand == "cargo test" &&
					info.LintCommand == "cargo clippy"
			},
		},
		{
			name: "Project with Taskfile",
			setup: func(t *testing.T, dir string) {
				goModContent := `module example.com/testproject
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
      - golangci-lint run
`
				err = os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(taskfileContent), 0o644)
				require.NoError(t, err)
			},
			expected: func(info CodebaseInfo) bool {
				return info.BuildCommand == "task build" &&
					info.TestCommand == "task test" &&
					info.LintCommand == "task lint" &&
					contains(info.ConfigFiles, "Taskfile.yml")
			},
		},
		{
			name: "Monorepo with lerna.json",
			setup: func(t *testing.T, dir string) {
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
			expected: func(info CodebaseInfo) bool {
				return info.ProjectType == "monorepo"
			},
		},
		{
			name: "Service with API directory",
			setup: func(t *testing.T, dir string) {
				err := os.MkdirAll(filepath.Join(dir, "api"), 0o755)
				require.NoError(t, err)

				goModContent := `module example.com/api-service
go 1.21`
				err = os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0o644)
				require.NoError(t, err)
			},
			expected: func(info CodebaseInfo) bool {
				return info.ProjectType == "service"
			},
		},
		{
			name: "Project with database",
			setup: func(t *testing.T, dir string) {
				dockerComposeContent := `version: '3.8'

services:
  postgres:
    image: postgres:15
    environment:
      POSTGRES_PASSWORD: password
`
				err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(dockerComposeContent), 0o644)
				require.NoError(t, err)
			},
			expected: func(info CodebaseInfo) bool {
				return info.HasDatabase && info.HasDocker
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

			tt.setup(t, dir)

			info := AnalyzeCodebase("TestProject")

			assert.Equal(t, "TestProject", info.ProjectName)
			assert.True(t, tt.expected(info), "Expectation failed for %s: %+v", tt.name, info)
		})
	}
}

func TestDetectMCPCapability(t *testing.T) {
	info := CodebaseInfo{}
	detectMCPCapability(&info)

	if info.HasMCP {
		assert.NotEmpty(t, info.MCPCommand)
		assert.Contains(t, []string{"uvx", "npx", "go run"}, info.MCPCommand)
	}
}

func TestDetectProjectType(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T, dir string)
		info     CodebaseInfo
		expected string
	}{
		{
			name: "Library project",
			setup: func(t *testing.T, dir string) {
				setupContent := `from setuptools import setup
setup(name='my-lib')`
				err := os.WriteFile(filepath.Join(dir, "setup.py"), []byte(setupContent), 0o644)
				require.NoError(t, err)
			},
			info:     CodebaseInfo{ProjectName: "my-lib"},
			expected: "library",
		},
		{
			name: "Default to application",
			setup: func(t *testing.T, dir string) {
			},
			info:     CodebaseInfo{ProjectName: "my-app"},
			expected: "application",
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

			tt.setup(t, dir)
			detectProjectType(&tt.info)
			assert.Equal(t, tt.expected, tt.info.ProjectType)
		})
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
