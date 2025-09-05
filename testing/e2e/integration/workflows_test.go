package integration

import (
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/testing/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type WorkflowsTestSuite struct {
	suite.Suite
	workingDir string
}

func TestWorkflowsSuite(t *testing.T) {
	suite.Run(t, new(WorkflowsTestSuite))
}

func (s *WorkflowsTestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
}

func (s *WorkflowsTestSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

func (s *WorkflowsTestSuite) TestCompleteProjectLifecycle() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "WorkflowTest", "--preset", "claude")
	result.AssertStderrContains(s.T(), "Created ai_rulez.yaml")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	s.True(testutil.FileExists(s.T(), configPath))

	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")
	result.AssertOutputContains(s.T(), "valid")

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"Custom Workflow Rule",
		"--content", "Custom workflow rule",
		"--priority", "high")

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "agent", "workflow-agent",
		"--description", "Agent for workflow testing",
		"--tools", "Read,Edit")

	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")
	result.AssertOutputContains(s.T(), "valid")

	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	claudePath := filepath.Join(s.workingDir, "CLAUDE.md")
	s.True(testutil.FileExists(s.T(), claudePath))

	agentsDir := filepath.Join(s.workingDir, ".claude", "agents")
	s.True(testutil.FileExists(s.T(), agentsDir))

	agentPath := filepath.Join(agentsDir, "workflow-agent.md")
	s.True(testutil.FileExists(s.T(), agentPath))

	claudeContent := testutil.ReadFile(s.T(), claudePath)
	s.Contains(claudeContent, "WorkflowTest")
	s.Contains(claudeContent, "Custom workflow rule")
	s.Contains(claudeContent, "Follow the project's established coding conventions")

	agentContent := testutil.ReadFile(s.T(), agentPath)
	s.Contains(agentContent, "workflow-agent")
	s.Contains(agentContent, "Agent for workflow testing")
}

func (s *WorkflowsTestSuite) TestMultiProviderWorkflow() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "MultiProvider", "--popular")
	result.AssertStderrContains(s.T(), "Claude")
	result.AssertStderrContains(s.T(), "Cursor")
	result.AssertStderrContains(s.T(), "Windsurf")
	result.AssertStderrContains(s.T(), "Copilot")

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"Claude Rule",
		"--content", "Claude-specific rule")

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"Cursor Rule",
		"--content", "Cursor-specific rule")

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"Universal Rule",
		"--content", "Universal rule for all providers")

	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	claudePath := filepath.Join(s.workingDir, "CLAUDE.md")
	claudeContent := testutil.ReadFile(s.T(), claudePath)
	s.Contains(claudeContent, "Claude-specific rule")
	s.Contains(claudeContent, "Universal rule")
	s.Contains(claudeContent, "Cursor-specific rule")

	cursorPath := filepath.Join(s.workingDir, ".cursor", "rules")
	s.True(testutil.FileExists(s.T(), cursorPath))
}

func (s *WorkflowsTestSuite) TestCRUDWorkflow() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"CRUD Test Rule",
		"--content", "CRUD test rule")
	result.AssertOutputContains(s.T(), "Added rule")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "CRUD test rule")

	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "rule",
		"CRUD Test Rule",
		"--content", "Updated CRUD rule",
		"--priority", "critical")
	result.AssertOutputContains(s.T(), "Updated rule")

	content = testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "Updated CRUD rule")
	s.NotContains(content, "CRUD test rule")

	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "rule", "CRUD Test Rule")
	result.AssertOutputContains(s.T(), "Deleted rule")

	content = testutil.ReadFile(s.T(), configPath)
	s.NotContains(content, "Updated CRUD rule")
}

func (s *WorkflowsTestSuite) TestErrorRecoveryWorkflow() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")
	result.AssertOutputContains(s.T(), "valid")

	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.InvalidYAMLConfig)

	result = testutil.RunCLIExpectError(s.T(), s.workingDir, "validate")
	result.AssertStderrContains(s.T(), "invalid")

	result = testutil.RunCLIExpectError(s.T(), s.workingDir, "generate")
	result.AssertStderrContains(s.T(), "Error")

	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)

	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")
	result.AssertOutputContains(s.T(), "valid")

	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")
}

func (s *WorkflowsTestSuite) TestConfigEvolutionWorkflow() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.MinimalConfig)

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	initialOutput := testutil.ReadFile(s.T(), filepath.Join(s.workingDir, "output.md"))

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "output", "CLAUDE.md")

	configPath3 := filepath.Join(s.workingDir, "ai_rulez.yaml")
	config3 := testutil.ReadFile(s.T(), configPath3)
	config3 += `
sections:
  - name: "New Guidelines"
    content: "Added after initial setup"
    priority: medium
`
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", config3)

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"Additional Rule",
		"--content", "Additional rule for evolved config")

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")
	result.AssertOutputContains(s.T(), "valid")

	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	s.True(testutil.FileExists(s.T(), filepath.Join(s.workingDir, "CLAUDE.md")))

	claudeContent := testutil.ReadFile(s.T(), filepath.Join(s.workingDir, "CLAUDE.md"))
	s.Contains(claudeContent, "Added after initial setup")
	s.Contains(claudeContent, "Additional rule")

	s.True(testutil.FileExists(s.T(), filepath.Join(s.workingDir, "output.md")))

	updatedOutput := testutil.ReadFile(s.T(), filepath.Join(s.workingDir, "output.md"))
	s.NotEqual(initialOutput, updatedOutput, "Output should be updated with new content")
}
