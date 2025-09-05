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
	lefthookContent := `pre-commit:\n  commands:\n    lint:\n      run: npm run lint\n`
	testutil.WriteFile(s.T(), s.workingDir, "lefthook.yml", lefthookContent)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "HookProject", "--setup-hooks")

	result.AssertStderrContains(s.T(), "Successfully configured Lefthook")

	modifiedContent := testutil.ReadFile(s.T(), filepath.Join(s.workingDir, "lefthook.yml"))
	s.Contains(modifiedContent, "ai-rulez validate", "The validate command should be added to lefthook.yml")
}

func (s *InitCLITestSuite) TestInitConflictingProviders() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "ConflictProject", "--claude", "--continue-dev")

	result.AssertStderrContains(s.T(), "Created ai_rulez.yaml")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)

	s.Equal(1, strings.Count(content, "# agents:"), "Should only be one agents block in the config")
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
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "ContinueDevProject", "--preset", "continue-dev")

	result.AssertStderrContains(s.T(), "Continue.dev rules")
	result.AssertStderrContains(s.T(), "Continue.dev prompts")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	configContent := testutil.ReadFile(s.T(), configPath)
	s.Contains(configContent, "path: \".continue/prompts/ai_rulez_prompts.yaml\"")
	s.Contains(configContent, "# agents:")
	s.Contains(configContent, "name: \"architect\"")

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	promptsPath := filepath.Join(s.workingDir, ".continue", "prompts", "ai_rulez_prompts.yaml")
	s.True(testutil.FileExists(s.T(), promptsPath), "prompts YAML file should be generated")
	promptsContent := testutil.ReadFile(s.T(), promptsPath)
	s.Contains(promptsContent, "GENERATED FILE - DO NOT EDIT DIRECTLY")

	secondWorkingDir := testutil.CreateTempDir(s.T())

	resultSecond := testutil.RunCLIExpectSuccess(s.T(), secondWorkingDir, "init", "ProjectSecond", "--preset", "continue-dev")

	resultSecond.AssertStderrContains(s.T(), "Continue.dev now uses YAML configuration")
	resultSecond.AssertStderrContains(s.T(), "Custom prompts will be generated in .continue/prompts/")
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
