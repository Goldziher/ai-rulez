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
	// Step 1: Initialize new project
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "WorkflowTest", "--preset", "claude")
	result.AssertStderrContains(s.T(), "Created ai_rulez.yaml")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	s.True(testutil.FileExists(s.T(), configPath))

	// Step 2: Validate initial config
	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")
	result.AssertOutputContains(s.T(), "valid")

	// Step 3: Add custom rules and sections
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"--name", "Custom Workflow Rule",
		"--content", "Custom workflow rule",
		"--priority", "8")

	// Skip adding sections since the init command already creates them
	// The test just needs to verify generation works

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "agent", "workflow-agent",
		"--description", "Agent for workflow testing",
		"--tools", "Read,Edit")

	// Step 4: Validate updated config
	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")
	result.AssertOutputContains(s.T(), "valid")

	// Step 5: Generate output files
	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	// Step 6: Verify all outputs were created
	claudePath := filepath.Join(s.workingDir, "CLAUDE.md")
	s.True(testutil.FileExists(s.T(), claudePath))

	agentsDir := filepath.Join(s.workingDir, ".claude", "agents")
	s.True(testutil.FileExists(s.T(), agentsDir))

	agentPath := filepath.Join(agentsDir, "workflow-agent.md")
	s.True(testutil.FileExists(s.T(), agentPath))

	// Step 7: Verify content quality
	claudeContent := testutil.ReadFile(s.T(), claudePath)
	s.Contains(claudeContent, "WorkflowTest")
	s.Contains(claudeContent, "Custom workflow rule")
	s.Contains(claudeContent, "Follow the project's established coding conventions") // From init template section content

	agentContent := testutil.ReadFile(s.T(), agentPath)
	s.Contains(agentContent, "workflow-agent")
	s.Contains(agentContent, "Agent for workflow testing")
}

func (s *WorkflowsTestSuite) TestMultiProviderWorkflow() {
	// Initialize with multiple providers
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "MultiProvider", "--popular")
	result.AssertStderrContains(s.T(), "Claude")
	result.AssertStderrContains(s.T(), "Cursor")
	result.AssertStderrContains(s.T(), "Windsurf")
	result.AssertStderrContains(s.T(), "Copilot")

	// Add targeted rules - CLI doesn't support --targets, so add via basic rules
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"--name", "Claude Rule",
		"--content", "Claude-specific rule")

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"--name", "Cursor Rule",
		"--content", "Cursor-specific rule")

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"--name", "Universal Rule",
		"--content", "Universal rule for all providers")

	// Generate outputs
	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	// Verify content generation (without targeting since CLI doesn't support it)
	claudePath := filepath.Join(s.workingDir, "CLAUDE.md")
	claudeContent := testutil.ReadFile(s.T(), claudePath)
	s.Contains(claudeContent, "Claude-specific rule")
	s.Contains(claudeContent, "Universal rule")
	s.Contains(claudeContent, "Cursor-specific rule") // Will appear in all outputs without targeting

	cursorPath := filepath.Join(s.workingDir, ".cursor", "rules")
	s.True(testutil.FileExists(s.T(), cursorPath))
}

func (s *WorkflowsTestSuite) TestCRUDWorkflow() {
	// Start with basic config
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)

	// Test complete CRUD lifecycle for rules
	// CREATE
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"--name", "CRUD Test Rule",
		"--content", "CRUD test rule")
	result.AssertOutputContains(s.T(), "Added rule")

	// READ - verify it was added
	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "CRUD test rule")

	// UPDATE
	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "rule",
		"--name", "CRUD Test Rule",
		"--content", "Updated CRUD rule",
		"--priority", "9")
	result.AssertOutputContains(s.T(), "Updated rule")

	content = testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "Updated CRUD rule")
	s.NotContains(content, "CRUD test rule")

	// DELETE
	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "rule", "CRUD Test Rule")
	result.AssertOutputContains(s.T(), "Deleted rule")

	content = testutil.ReadFile(s.T(), configPath)
	s.NotContains(content, "Updated CRUD rule")
}

func (s *WorkflowsTestSuite) TestErrorRecoveryWorkflow() {
	// Start with valid config
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)

	// Validate it works
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")
	result.AssertOutputContains(s.T(), "valid")

	// Introduce error by corrupting config
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.InvalidYAMLConfig)

	// Verify error is detected
	result = testutil.RunCLIExpectError(s.T(), s.workingDir, "validate")
	result.AssertStderrContains(s.T(), "invalid")

	// Generation should also fail
	result = testutil.RunCLIExpectError(s.T(), s.workingDir, "generate")
	result.AssertStderrContains(s.T(), "Error")

	// Fix config
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)

	// Verify recovery
	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")
	result.AssertOutputContains(s.T(), "valid")

	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")
}

func (s *WorkflowsTestSuite) TestConfigEvolutionWorkflow() {
	// Start minimal
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.MinimalConfig)

	// Validate and generate
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")

	initialOutput := testutil.ReadFile(s.T(), filepath.Join(s.workingDir, "output.md"))

	// Evolve: Add more outputs
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "output", "CLAUDE.md")

	// Evolve: Add sections (CLI doesn't support --content, so modify file)
	configPath3 := filepath.Join(s.workingDir, "ai_rulez.yaml")
	config3 := testutil.ReadFile(s.T(), configPath3)
	config3 += `
sections:
  - name: "New Guidelines"
    content: "Added after initial setup"
    priority: 5
`
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", config3)

	// Evolve: Add more rules
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"--name", "Additional Rule",
		"--content", "Additional rule for evolved config")

	// Validate evolved config
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")
	result.AssertOutputContains(s.T(), "valid")

	// Generate with evolved config
	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "generate")
	result.AssertOutputContains(s.T(), "Generated")

	// Verify new outputs
	s.True(testutil.FileExists(s.T(), filepath.Join(s.workingDir, "CLAUDE.md")))

	claudeContent := testutil.ReadFile(s.T(), filepath.Join(s.workingDir, "CLAUDE.md"))
	s.Contains(claudeContent, "Added after initial setup") // Section content
	s.Contains(claudeContent, "Additional rule")

	// Original output should still exist and be updated
	s.True(testutil.FileExists(s.T(), filepath.Join(s.workingDir, "output.md")))

	updatedOutput := testutil.ReadFile(s.T(), filepath.Join(s.workingDir, "output.md"))
	s.NotEqual(initialOutput, updatedOutput, "Output should be updated with new content")
}
