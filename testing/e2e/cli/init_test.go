package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/testing/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type InitCLITestSuite struct {
	suite.Suite
	workingDir string
}

func TestInitCLISuite(t *testing.T) {
	suite.Run(t, new(InitCLITestSuite))
}

func (s *InitCLITestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
}

func (s *InitCLITestSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

func (s *InitCLITestSuite) TestInitSetupHooks() {
	// Create a dummy lefthook.yml file
	lefthookContent := `pre-commit:\n  commands:\n    lint:\n      run: npm run lint\n`
	testutil.WriteFile(s.T(), s.workingDir, "lefthook.yml", lefthookContent)

	// Run init with the --setup-hooks flag
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "HookProject", "--setup-hooks")

	// Check that the CLI reported success
	result.AssertStderrContains(s.T(), "Successfully configured Lefthook")

	// Read the modified lefthook.yml and verify the change
	modifiedContent := testutil.ReadFile(s.T(), filepath.Join(s.workingDir, "lefthook.yml"))
	s.Contains(modifiedContent, "ai-rulez validate", "The validate command should be added to lefthook.yml")
}

func (s *InitCLITestSuite) TestInitConflictingProviders() {
	// Run init with two providers that both want to create an 'agents' block
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "ConflictProject", "--claude", "--continue-dev")

	result.AssertStderrContains(s.T(), "Created ai_rulez.yaml")

	// Read the generated config
	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)

	// 1. Assert that there is only one 'agents:' block
	s.Equal(1, strings.Count(content, "agents:"), "Should only be one agents block in the config")

	// 2. Assert that the Claude configuration took precedence
	s.Contains(content, "# AI agents (specialized sub-assistants for Claude)")
	s.Contains(content, "model: \"claude-3-opus-20240229\"")
	s.NotContains(content, "model: \"gpt-4-turbo\"")
}

func (s *InitCLITestSuite) TestBasicInit() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "TestProject")

	result.AssertStderrContains(s.T(), "Created ai_rulez.yaml")
	result.AssertStderrContains(s.T(), "TestProject")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	s.True(testutil.FileExists(s.T(), configPath), "Config file should be created")

	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "name: \"TestProject\"")
	s.Contains(content, "CLAUDE.md")
}

func (s *InitCLITestSuite) TestInitContinueDevPreset() {
	// Part 1: Test with no existing config.py
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "ContinueDevProject", "--preset", "continue-dev")

	// Check for correct output during init
	result.AssertStderrContains(s.T(), "Continue.dev rules")
	result.AssertStderrContains(s.T(), "Continue.dev agents")
	result.AssertStderrContains(s.T(), "Created sample config.py")

	// Check that ai_rulez.yaml is correct
	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	configContent := testutil.ReadFile(s.T(), configPath)
	s.Contains(configContent, "path: \".continue/ai_rulez_agents.py\"")
	s.Contains(configContent, "template: \"continuedev-agents\"")
	s.Contains(configContent, "agents:")
	s.Contains(configContent, "name: \"code-reviewer\"")

	// Check that sample config.py was created
	configPyPath := filepath.Join(s.workingDir, "config.py")
	s.True(testutil.FileExists(s.T(), configPyPath), "Sample config.py should be created")
	configPyContent := testutil.ReadFile(s.T(), configPyPath)
	s.Contains(configPyContent, "from .continue.ai_rulez_agents import generated_agents")
	s.Contains(configPyContent, "custom_llms=generated_agents")

	// Run generate and check for the generated agents file
	genResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	genResult.AssertStderrContains(s.T(), "Generated 1 file(s)")

	agentsPyPath := filepath.Join(s.workingDir, ".continue", "ai_rulez_agents.py")
	s.True(testutil.FileExists(s.T(), agentsPyPath), "agents.py file should be generated")
	agentsPyContent := testutil.ReadFile(s.T(), agentsPyPath)
	s.Contains(agentsPyContent, "GENERATED FILE - DO NOT EDIT DIRECTLY")
	s.Contains(agentsPyContent, "title=\"code-reviewer\"")
	s.Contains(agentsPyContent, "model=\"gpt-4-turbo\"")

	// Part 2: Test with an existing config.py
	secondWorkingDir := testutil.CreateTempDir(s.T())
	testutil.WriteFile(s.T(), secondWorkingDir, "config.py", "# Existing user config")

	resultWithExisting := testutil.RunCLIExpectSuccess(s.T(), secondWorkingDir, "init", "ProjectWithExisting", "--preset", "continue-dev")

	// Check that it printed instructions instead of creating the file
	resultWithExisting.AssertStderrContains(s.T(), "Found existing config.py")
	resultWithExisting.AssertStderrContains(s.T(), "1. Import the generated agents")
	resultWithExisting.AssertStderrContains(s.T(), "2. Add the imported agents")

	// Verify it didn't overwrite the file
	existingContent := testutil.ReadFile(s.T(), filepath.Join(secondWorkingDir, "config.py"))
	s.Equal("# Existing user config", existingContent)
}

func (s *InitCLITestSuite) TestInitWithoutProjectName() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init")

	result.AssertStderrContains(s.T(), "Created ai_rulez.yaml")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "name:")
}

func (s *InitCLITestSuite) TestInitClaudePreset() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "ClaudeProject", "--preset", "claude")

	result.AssertStderrContains(s.T(), "Claude (CLAUDE.md)")
	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "CLAUDE.md")
	s.Contains(content, ".claude/agents/")
}

func (s *InitCLITestSuite) TestInitCursorPreset() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "CursorProject", "--preset", "cursor")

	result.AssertStderrContains(s.T(), "Cursor (.cursor/rules/)")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, ".cursor/rules/")
}

func (s *InitCLITestSuite) TestInitWindsurfPreset() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "WindsurfProject", "--preset", "windsurf")

	result.AssertStderrContains(s.T(), "Windsurf (.windsurf/)")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, ".windsurf/")
}

func (s *InitCLITestSuite) TestInitPopularProviders() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "PopularProject", "--popular")

	result.AssertStderrContains(s.T(), "Claude (CLAUDE.md)")
	result.AssertStderrContains(s.T(), "Cursor (.cursor/rules/)")
	result.AssertStderrContains(s.T(), "Windsurf (.windsurf/)")
	result.AssertStderrContains(s.T(), "GitHub Copilot")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "CLAUDE.md")
	s.Contains(content, ".cursor/rules/")
	s.Contains(content, ".windsurf/")
	s.Contains(content, ".github/copilot-instructions.md")
}

func (s *InitCLITestSuite) TestInitAllProviders() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "AllProject", "--all")

	result.AssertStderrContains(s.T(), "Claude")
	result.AssertStderrContains(s.T(), "Cursor")
	result.AssertStderrContains(s.T(), "Windsurf")
	result.AssertStderrContains(s.T(), "Copilot")
	result.AssertStderrContains(s.T(), "Gemini")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "CLAUDE.md")
	s.Contains(content, "GEMINI.md")
	s.Contains(content, "AGENTS.md")
}

func (s *InitCLITestSuite) TestInitIndividualProviders() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "CustomProject", "--claude", "--cursor")

	result.AssertStderrContains(s.T(), "Claude (CLAUDE.md)")
	result.AssertStderrContains(s.T(), "Cursor (.cursor/rules/)")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "CLAUDE.md")
	s.Contains(content, ".cursor/rules/")
	s.NotContains(content, ".windsurf/")
}

func (s *InitCLITestSuite) TestInitNoComments() {
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "NoCommentsProject", "--no-comments")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)

	commentLines := strings.Count(content, "# ")
	s.LessOrEqual(commentLines, 5, "Should have minimal comments with --no-comments flag")
}

func (s *InitCLITestSuite) TestInitExistingConfig() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", "existing: config")

	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "init", "TestProject")
	result.AssertStderrContains(s.T(), "Configuration file already exists")
}

func (s *InitCLITestSuite) TestInitListAgents() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "--list-agents")

	result.AssertOutputContains(s.T(), "Available")
}

func (s *InitCLITestSuite) TestInitNoAgent() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "NoAgentProject", "--no-agent")

	result.AssertStderrContains(s.T(), "Created ai_rulez.yaml")
}

func (s *InitCLITestSuite) TestInitInvalidPreset() {
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "InvalidPresetProject", "--preset", "nonexistent")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	s.True(testutil.FileExists(s.T(), configPath))
}
