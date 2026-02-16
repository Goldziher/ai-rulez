package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildAgentPrompt(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		config      templates.ProviderConfig
		setup       func(t *testing.T, dir string)
		checkPrompt func(t *testing.T, prompt string)
	}{
		{
			name:        "Claude configuration",
			projectName: "TestProject",
			config:      templates.ProviderConfig{Claude: true},
			setup: func(t *testing.T, dir string) {
				goModContent := `module example.com/testproject
go 1.21`
				err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0o644)
				require.NoError(t, err)
			},
			checkPrompt: func(t *testing.T, prompt string) {
				assert.Contains(t, prompt, "KEEP MINIMAL")
				assert.Contains(t, prompt, "SELECTIVELY UNCOMMENT")
				assert.Contains(t, prompt, "TestProject")
				assert.Contains(t, prompt, "CLAUDE.md")
				assert.Contains(t, prompt, "Tech Stack: Go")
				assert.Contains(t, prompt, "Build Command: go build")
			},
		},
		{
			name:        "Cursor configuration",
			projectName: "CursorProject",
			config:      templates.ProviderConfig{Cursor: true},
			setup: func(t *testing.T, dir string) {
				packageJSON := `{
  "name": "cursor-project",
  "scripts": {
    "build": "tsc",
    "test": "jest"
  },
  "devDependencies": {
    "typescript": "^5.0.0"
  }
}`
				err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(packageJSON), 0o644)
				require.NoError(t, err)
			},
			checkPrompt: func(t *testing.T, prompt string) {
				assert.Contains(t, prompt, ".cursorrules")
				assert.Contains(t, prompt, "Tech Stack: TypeScript")
				assert.Contains(t, prompt, "Build Command: npm run build")
			},
		},
		{
			name:        "Multiple providers",
			projectName: "MultiProject",
			config: templates.ProviderConfig{
				Claude: true,
				Gemini: true,
				Amp:    true,
			},
			setup: func(t *testing.T, dir string) {
			},
			checkPrompt: func(t *testing.T, prompt string) {
				assert.Contains(t, prompt, "CLAUDE.md")
				assert.Contains(t, prompt, "VALIDATE YOUR YAML")
			},
		},
		{
			name:        "Project with database",
			projectName: "DatabaseProject",
			config:      templates.ProviderConfig{Claude: true},
			setup: func(t *testing.T, dir string) {
				dockerComposeContent := `version: '3.8'
services:
  postgres:
    image: postgres:15`
				err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(dockerComposeContent), 0o644)
				require.NoError(t, err)
			},
			checkPrompt: func(t *testing.T, prompt string) {
				assert.Contains(t, prompt, "Has Database: true")
				assert.Contains(t, prompt, "Has Docker: true")
			},
		},
		{
			name:        "Monorepo project",
			projectName: "MonorepoProject",
			config:      templates.ProviderConfig{Claude: true},
			setup: func(t *testing.T, dir string) {
				lernaContent := `{"packages": ["packages/*"]}`
				err := os.WriteFile(filepath.Join(dir, "lerna.json"), []byte(lernaContent), 0o644)
				require.NoError(t, err)
			},
			checkPrompt: func(t *testing.T, prompt string) {
				assert.Contains(t, prompt, "Project Type: monorepo")
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

			prompt := buildAgentPrompt(tt.projectName, tt.config)

			tt.checkPrompt(t, prompt)
		})
	}
}

func TestPromptInstructions(t *testing.T) {
	dir := t.TempDir()
	oldDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(dir)
	require.NoError(t, err)
	defer os.Chdir(oldDir)

	prompt := buildAgentPrompt("TestProject", templates.ProviderConfig{Claude: true})

	essentialInstructions := []string{
		"INSTRUCTIONS:",
		"1. KEEP MINIMAL:",
		"2. SELECTIVELY UNCOMMENT",
		"4. VALIDATE YOUR YAML:",
		"PROJECT ANALYSIS:",
		"CONFIGURATION TEMPLATE:",
		"v2.0",
		"# AI-Rulez Configuration v2.0",
		"metadata:",
		"outputs:",
	}

	for _, instruction := range essentialInstructions {
		assert.Contains(t, prompt, instruction, "Missing essential instruction: %s", instruction)
	}
}

func TestPromptWithRealAITools(t *testing.T) {
	aiTools := []struct {
		name   string
		config templates.ProviderConfig
	}{
		{"Claude", templates.ProviderConfig{Claude: true}},
		{"Cursor", templates.ProviderConfig{Cursor: true}},
		{"Gemini", templates.ProviderConfig{Gemini: true}},
		{"Amp", templates.ProviderConfig{Amp: true}},
		{"Codex", templates.ProviderConfig{Codex: true}},
		{"Windsurf", templates.ProviderConfig{Windsurf: true}},
		{"Copilot", templates.ProviderConfig{Copilot: true}},
		{"Cline", templates.ProviderConfig{Cline: true}},
		{"ContinueDev", templates.ProviderConfig{ContinueDev: true}},
	}

	dir := t.TempDir()
	oldDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(dir)
	require.NoError(t, err)
	defer os.Chdir(oldDir)

	goModContent := `module example.com/testproject
go 1.21`
	err = os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0o644)
	require.NoError(t, err)

	packageJSON := `{
  "name": "test-project",
  "scripts": {"build": "tsc"}
}`
	err = os.WriteFile(filepath.Join(dir, "package.json"), []byte(packageJSON), 0o644)
	require.NoError(t, err)

	for _, tool := range aiTools {
		t.Run(tool.name, func(t *testing.T) {
			prompt := buildAgentPrompt("TestProject", tool.config)

			assert.True(t, strings.Contains(prompt, "metadata:"))
			assert.True(t, strings.Contains(prompt, "outputs:"))

			switch {
			case tool.config.Claude:
				assert.Contains(t, prompt, "CLAUDE.md")
			case tool.config.Cursor:
				assert.Contains(t, prompt, ".cursorrules")
			case tool.config.Windsurf:
				assert.Contains(t, prompt, ".windsurf/rules/")
			}

			assert.Contains(t, prompt, "# agents:")
			assert.Contains(t, prompt, "# rules:")
			assert.Contains(t, prompt, "# sections:")
			assert.Contains(t, prompt, "# commands:")
		})
	}
}

func TestSelectAgentUsesConfiguredAgentWhenSingleMatch(t *testing.T) {
	dir := t.TempDir()
	createDummyCommand(t, dir, "codex")
	t.Setenv("PATH", dir)

	selected := selectAgent("", templates.ProviderConfig{Codex: true}, true, true)
	require.NotNil(t, selected)
	assert.Equal(t, "codex", selected.ID)
}

func TestSelectAgentPromptsWhenMultipleMatches(t *testing.T) {
	dir := t.TempDir()
	createDummyCommand(t, dir, "amp")
	createDummyCommand(t, dir, "gemini")
	t.Setenv("PATH", dir)

	r, w, err := os.Pipe()
	require.NoError(t, err)

	originalStdin := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = originalStdin
	}()

	go func() {
		_, _ = w.Write([]byte("2\n"))
		_ = w.Close()
	}()

	selected := selectAgent("", templates.ProviderConfig{Amp: true, Gemini: true}, true, false)
	require.NotNil(t, selected)
	assert.Equal(t, "gemini", selected.ID)
}

func TestSelectAgentAutoYesDefaultsToFirstOption(t *testing.T) {
	dir := t.TempDir()
	createDummyCommand(t, dir, "amp")
	createDummyCommand(t, dir, "gemini")
	t.Setenv("PATH", dir)

	selected := selectAgent("", templates.ProviderConfig{Amp: true, Gemini: true}, true, true)
	require.NotNil(t, selected)
	assert.Equal(t, "amp", selected.ID)
}

func TestSelectAgentPromptsWhenProvidersInferred(t *testing.T) {
	dir := t.TempDir()
	createDummyCommand(t, dir, "claude")
	createDummyCommand(t, dir, "cursor-agent")
	t.Setenv("PATH", dir)

	r, w, err := os.Pipe()
	require.NoError(t, err)

	originalStdin := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = originalStdin
	}()

	go func() {
		_, _ = w.Write([]byte("2\n"))
		_ = w.Close()
	}()

	config := templates.ProviderConfig{Claude: true}

	selected := selectAgent("", config, false, false)
	require.NotNil(t, selected)
	assert.Equal(t, "cursor", selected.ID)
}

func createDummyCommand(t *testing.T, dir, name string) {
	t.Helper()

	var path string
	var script []byte

	// On Windows, we need to create a batch file or executable
	if strings.Contains(strings.ToLower(os.Getenv("OS")), "windows") || filepath.Ext(os.Args[0]) == ".exe" {
		path = filepath.Join(dir, name+".bat")
		script = []byte("@echo off\r\nexit /b 0\r\n")
	} else {
		path = filepath.Join(dir, name)
		script = []byte("#!/bin/sh\nexit 0\n")
	}

	err := os.WriteFile(path, script, 0o755)
	require.NoError(t, err)
}
