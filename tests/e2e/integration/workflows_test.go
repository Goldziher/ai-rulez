package integration

import (
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/tests/e2e/testutil"
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
	// Initialize V3 project
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "WorkflowTest", "--yes")
	result.AssertOutputContains(s.T(), "Created .ai-rulez/")

	configPath := filepath.Join(s.workingDir, ".ai-rulez", "config.yaml")
	s.True(testutil.FileExists(s.T(), configPath))

	// Add a custom rule
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"Custom Workflow Rule",
		"--content", "Custom workflow rule",
		"--priority", "high")

	// Verify rule was added
	ruleFile := filepath.Join(s.workingDir, ".ai-rulez", "rules", "Custom Workflow Rule.md")
	s.True(testutil.FileExists(s.T(), ruleFile))

	// Add a skill
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "skill",
		"workflow-agent",
		"--description", "Agent for workflow testing")

	skillDir := filepath.Join(s.workingDir, ".ai-rulez", "skills", "workflow-agent")
	s.True(testutil.FileExists(s.T(), skillDir))

	skillFile := filepath.Join(skillDir, "SKILL.md")
	s.True(testutil.FileExists(s.T(), skillFile))

	skillContent := testutil.ReadFile(s.T(), skillFile)
	s.Contains(skillContent, "Workflow Agent")

	ruleContent := testutil.ReadFile(s.T(), ruleFile)
	s.Contains(ruleContent, "Custom workflow rule")
}

func (s *WorkflowsTestSuite) TestMultiProviderWorkflow() {
	// Initialize V3 project with multiple presets
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "MultiProvider", "--yes")
	result.AssertOutputContains(s.T(), "Created .ai-rulez/")

	configPath := filepath.Join(s.workingDir, ".ai-rulez", "config.yaml")
	s.True(testutil.FileExists(s.T(), configPath))

	// Update config to include multiple presets
	currentConfig := testutil.ReadFile(s.T(), configPath)
	updatedConfig := currentConfig + `
# Add multiple presets
presets:
  - claude
  - cursor
`
	testutil.WriteFile(s.T(), filepath.Join(s.workingDir, ".ai-rulez"), "config.yaml", updatedConfig)

	// Add rules for different providers
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"Claude Rule",
		"--content", "Claude-specific rule")

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"Cursor Rule",
		"--content", "Cursor-specific rule")

	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"Universal Rule",
		"--content", "Universal rule for all providers")

	// Verify rules were added
	ruleDir := filepath.Join(s.workingDir, ".ai-rulez", "rules")
	s.True(testutil.FileExists(s.T(), filepath.Join(ruleDir, "Claude Rule.md")))
	s.True(testutil.FileExists(s.T(), filepath.Join(ruleDir, "Cursor Rule.md")))
	s.True(testutil.FileExists(s.T(), filepath.Join(ruleDir, "Universal Rule.md")))
}

func (s *WorkflowsTestSuite) TestCRUDWorkflow() {
	// Setup V3 basic config
	testutil.SetupV3BasicConfig(s.T(), s.workingDir)

	// Add a new rule
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"CRUD Test Rule",
		"--content", "CRUD test rule")
	result.AssertOutputContains(s.T(), "Rule added successfully")

	ruleFile := filepath.Join(s.workingDir, ".ai-rulez", "rules", "CRUD Test Rule.md")
	s.True(testutil.FileExists(s.T(), ruleFile))

	content := testutil.ReadFile(s.T(), ruleFile)
	s.Contains(content, "CRUD test rule")

	// Update the rule by removing and re-adding with new content
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "remove", "rule", "CRUD Test Rule", "--force")

	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"CRUD Test Rule",
		"--content", "Updated CRUD rule",
		"--priority", "critical")
	result.AssertOutputContains(s.T(), "Rule added successfully")

	content = testutil.ReadFile(s.T(), ruleFile)
	s.Contains(content, "Updated CRUD rule")
	s.Contains(content, "critical")

	// Delete the rule
	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "remove", "rule", "CRUD Test Rule", "--force")
	result.AssertOutputContains(s.T(), "Rule removed successfully")

	s.False(testutil.FileExists(s.T(), ruleFile))
}

func (s *WorkflowsTestSuite) TestErrorRecoveryWorkflow() {
	// Setup valid V3 config
	testutil.SetupV3BasicConfig(s.T(), s.workingDir)

	// Validate should succeed
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")
	result.AssertOutputContains(s.T(), "valid")

	// Make config invalid by corrupting it
	testutil.WriteFile(s.T(), filepath.Join(s.workingDir, ".ai-rulez"), "config.yaml", `version: "3.0"
name: "broken"
presets: not-a-list`)

	// Validate should fail
	result = testutil.RunCLIExpectError(s.T(), s.workingDir, "validate")
	result.AssertOutputContains(s.T(), "Error")

	// Restore valid config
	testutil.SetupV3BasicConfig(s.T(), s.workingDir)

	// Validate should succeed again
	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")
	result.AssertOutputContains(s.T(), "valid")
}

func (s *WorkflowsTestSuite) TestConfigEvolutionWorkflow() {
	// Initialize with V3 minimal setup
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "init", "EvolutionTest", "--yes", "--skip-content")
	result.AssertOutputContains(s.T(), "Created .ai-rulez/")

	// Validate and verify initial setup
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")

	// Add context to the project
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "context",
		"Project Guidelines",
		"--content", "Added after initial setup")

	// Verify context was added
	contextFile := filepath.Join(s.workingDir, ".ai-rulez", "context", "Project Guidelines.md")
	s.True(testutil.FileExists(s.T(), contextFile))

	contextContent := testutil.ReadFile(s.T(), contextFile)
	s.Contains(contextContent, "Added after initial setup")

	// Add additional rule
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"Additional Rule",
		"--content", "Additional rule for evolved config")

	// Verify rule was added
	ruleFile := filepath.Join(s.workingDir, ".ai-rulez", "rules", "Additional Rule.md")
	s.True(testutil.FileExists(s.T(), ruleFile))

	ruleContent := testutil.ReadFile(s.T(), ruleFile)
	s.Contains(ruleContent, "Additional rule for evolved config")

	// Validate config after evolution
	result = testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "validate")
	result.AssertOutputContains(s.T(), "valid")
}
