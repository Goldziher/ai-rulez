package commands_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/cmd/commands"
)

func TestInitCommand(t *testing.T) {
	assert.NotNil(t, commands.InitCmd)
	assert.Equal(t, "init [project-name]", commands.InitCmd.Use)

	flags := commands.InitCmd.Flags()

	// V3 flags
	assert.NotNil(t, flags.Lookup("format"))
	assert.NotNil(t, flags.Lookup("domains"))
	assert.NotNil(t, flags.Lookup("skip-content"))
	assert.NotNil(t, flags.Lookup("skip-mcp"))
	assert.NotNil(t, flags.Lookup("from"))
	assert.NotNil(t, flags.Lookup("setup-hooks"))
	assert.NotNil(t, flags.Lookup("yes"))
}

func TestInit_BasicStructure(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Set skip-content flag
	commands.InitCmd.Flags().Set("skip-content", "true")
	defer commands.InitCmd.Flags().Set("skip-content", "false")

	// Run init command
	commands.InitCmd.Run(commands.InitCmd, []string{"test-project"})

	// Assert basic structure was created
	assert.DirExists(t, ".ai-rulez")
	assert.DirExists(t, ".ai-rulez/rules")
	assert.DirExists(t, ".ai-rulez/context")
	assert.DirExists(t, ".ai-rulez/skills")
	assert.DirExists(t, ".ai-rulez/agents")
	assert.DirExists(t, ".ai-rulez/domains")

	// Assert config.yaml exists
	assert.FileExists(t, ".ai-rulez/config.yaml")

	// Read config content
	content, err := os.ReadFile(".ai-rulez/config.yaml")
	require.NoError(t, err)
	assert.Contains(t, string(content), "version: \"3.0\"")
	assert.Contains(t, string(content), "name: \"test-project\"")
	assert.Contains(t, string(content), "presets:")
}

func TestInit_WithDomains(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Set flags
	commands.InitCmd.Flags().Set("domains", "backend,frontend,qa")
	commands.InitCmd.Flags().Set("skip-content", "true")
	defer commands.InitCmd.Flags().Set("domains", "")
	defer commands.InitCmd.Flags().Set("skip-content", "false")

	// Run init command
	commands.InitCmd.Run(commands.InitCmd, []string{"test-project"})

	// Assert domain directories were created
	assert.DirExists(t, ".ai-rulez/domains/backend")
	assert.DirExists(t, ".ai-rulez/domains/backend/rules")
	assert.DirExists(t, ".ai-rulez/domains/backend/context")
	assert.DirExists(t, ".ai-rulez/domains/backend/skills")
	assert.DirExists(t, ".ai-rulez/domains/backend/agents")

	assert.DirExists(t, ".ai-rulez/domains/frontend")
	assert.DirExists(t, ".ai-rulez/domains/frontend/rules")
	assert.DirExists(t, ".ai-rulez/domains/frontend/context")
	assert.DirExists(t, ".ai-rulez/domains/frontend/skills")
	assert.DirExists(t, ".ai-rulez/domains/frontend/agents")

	assert.DirExists(t, ".ai-rulez/domains/qa")
	assert.DirExists(t, ".ai-rulez/domains/qa/rules")
	assert.DirExists(t, ".ai-rulez/domains/qa/context")
	assert.DirExists(t, ".ai-rulez/domains/qa/skills")
	assert.DirExists(t, ".ai-rulez/domains/qa/agents")
}

func TestInit_WithExampleContent(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Run init command (don't skip content)
	commands.InitCmd.Run(commands.InitCmd, []string{"test-project"})

	// Assert example content files exist
	assert.FileExists(t, ".ai-rulez/rules/code-quality.md")
	assert.FileExists(t, ".ai-rulez/context/architecture.md")
	assert.FileExists(t, ".ai-rulez/skills/code-reviewer/SKILL.md")

	// Verify content
	ruleContent, err := os.ReadFile(".ai-rulez/rules/code-quality.md")
	require.NoError(t, err)
	assert.Contains(t, string(ruleContent), "Code Quality Standards")
	assert.Contains(t, string(ruleContent), "priority: high")

	contextContent, err := os.ReadFile(".ai-rulez/context/architecture.md")
	require.NoError(t, err)
	assert.Contains(t, string(contextContent), "Project Architecture")

	skillContent, err := os.ReadFile(".ai-rulez/skills/code-reviewer/SKILL.md")
	require.NoError(t, err)
	assert.Contains(t, string(skillContent), "Code Reviewer")
	assert.Contains(t, string(skillContent), "name: code-reviewer")
}

func TestInit_JSONFormat(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Set flags
	commands.InitCmd.Flags().Set("format", "json")
	commands.InitCmd.Flags().Set("skip-content", "true")
	defer commands.InitCmd.Flags().Set("format", "yaml")
	defer commands.InitCmd.Flags().Set("skip-content", "false")

	// Run init command
	commands.InitCmd.Run(commands.InitCmd, []string{"test-project"})

	// Assert config.json exists (not config.yaml)
	assert.FileExists(t, ".ai-rulez/config.json")
	assert.NoFileExists(t, ".ai-rulez/config.yaml")

	// Verify JSON content
	content, err := os.ReadFile(".ai-rulez/config.json")
	require.NoError(t, err)
	assert.Contains(t, string(content), `"version": "3.0"`)
	assert.Contains(t, string(content), `"name": "test-project"`)
}

func TestInit_SkipContent(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Set flags
	commands.InitCmd.Flags().Set("skip-content", "true")
	defer commands.InitCmd.Flags().Set("skip-content", "false")

	// Run init command
	commands.InitCmd.Run(commands.InitCmd, []string{"test-project"})

	// Assert structure exists but no example files
	assert.DirExists(t, ".ai-rulez/rules")
	assert.DirExists(t, ".ai-rulez/context")
	assert.DirExists(t, ".ai-rulez/skills")
	assert.DirExists(t, ".ai-rulez/agents")

	// Verify no example content
	assert.NoFileExists(t, ".ai-rulez/rules/code-quality.md")
	assert.NoFileExists(t, ".ai-rulez/context/architecture.md")
	assert.NoDirExists(t, ".ai-rulez/skills/code-reviewer")
}

func TestInit_ExampleMCPServers(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Set flags
	commands.InitCmd.Flags().Set("skip-content", "true")
	defer commands.InitCmd.Flags().Set("skip-content", "false")

	// Run init command
	commands.InitCmd.Run(commands.InitCmd, []string{"test-project"})

	// Assert MCP file exists
	assert.FileExists(t, ".ai-rulez/mcp.yaml")

	content, err := os.ReadFile(".ai-rulez/mcp.yaml")
	require.NoError(t, err)
	assert.Contains(t, string(content), "name: ai-rulez")
	assert.Contains(t, string(content), "name: github")
	assert.Contains(t, string(content), "ai-rulez@latest")
}

func TestInit_ProjectNameFromDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	// Create a subdirectory with a specific name
	projectDir := filepath.Join(tmpDir, "my-awesome-project")
	err = os.MkdirAll(projectDir, 0o755)
	require.NoError(t, err)

	err = os.Chdir(projectDir)
	require.NoError(t, err)

	// Set flags
	commands.InitCmd.Flags().Set("skip-content", "true")
	defer commands.InitCmd.Flags().Set("skip-content", "false")

	// Run init command without project name argument
	commands.InitCmd.Run(commands.InitCmd, []string{})

	// Read config and verify project name is directory name
	content, err := os.ReadFile(".ai-rulez/config.yaml")
	require.NoError(t, err)
	assert.Contains(t, string(content), `name: "my-awesome-project"`)
}

func TestInit_DomainsWithSpaces(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Set flags with spaces in domains
	commands.InitCmd.Flags().Set("domains", "backend, frontend , qa")
	commands.InitCmd.Flags().Set("skip-content", "true")
	defer commands.InitCmd.Flags().Set("domains", "")
	defer commands.InitCmd.Flags().Set("skip-content", "false")

	// Run init command
	commands.InitCmd.Run(commands.InitCmd, []string{"test-project"})

	// Assert trimmed domain names were created
	assert.DirExists(t, ".ai-rulez/domains/backend")
	assert.DirExists(t, ".ai-rulez/domains/frontend")
	assert.DirExists(t, ".ai-rulez/domains/qa")
}
