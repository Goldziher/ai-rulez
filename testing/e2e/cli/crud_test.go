package cli

import (
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/testing/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type CRUDCLITestSuite struct {
	suite.Suite
	workingDir string
}

func TestCRUDCLISuite(t *testing.T) {
	suite.Run(t, new(CRUDCLITestSuite))
}

func (s *CRUDCLITestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
	// Create a basic config for CRUD operations
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)
}

func (s *CRUDCLITestSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

// ========== Rule CRUD Tests ==========

func (s *CRUDCLITestSuite) TestAddRule() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"--name", "Test Rule",
		"--content", "New test rule content",
		"--priority", "7")

	result.AssertStdoutContains(s.T(), "Added rule")

	// Verify rule was added to config
	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "New test rule content")
}

func (s *CRUDCLITestSuite) TestAddRuleWithName() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"--name", "Custom Rule Name",
		"--content", "Custom rule content",
		"--priority", "6")

	result.AssertOutputContains(s.T(), "Added rule")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "Custom Rule Name")
	s.Contains(content, "Custom rule content")
}

func (s *CRUDCLITestSuite) TestAddRuleWithPriority() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"--name", "Priority Rule",
		"--content", "Priority rule content",
		"--priority", "8")

	result.AssertStdoutContains(s.T(), "Added rule")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "Priority rule content")
}

func (s *CRUDCLITestSuite) TestUpdateRule() {
	// First add a rule
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule",
		"--name", "Original Rule",
		"--content", "Original content")

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "rule",
		"--name", "Original Rule",
		"--content", "Updated content",
		"--priority", "8")

	result.AssertStdoutContains(s.T(), "Updated rule")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "Updated content")
	s.NotContains(content, "Original content")
}

func (s *CRUDCLITestSuite) TestDeleteRule() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "rule", "Basic Rule")

	result.AssertStdoutContains(s.T(), "Deleted rule")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.NotContains(content, "Basic Rule") // Should remove the rule by name
}

func (s *CRUDCLITestSuite) TestDeleteRuleInvalidName() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "delete", "rule", "NonExistent Rule")

	result.AssertStdoutContains(s.T(), "not found")
}

// ========== Section CRUD Tests ==========

func (s *CRUDCLITestSuite) TestAddSection() {
	// Sections read content from stdin
	result := testutil.RunCLIExpectSuccessWithStdin(s.T(), s.workingDir, "New section content", "add", "section", "New Section", "--priority", "7")

	result.AssertStdoutContains(s.T(), "Added section")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "New Section")
	s.Contains(content, "New section content")
}

func (s *CRUDCLITestSuite) TestAddSectionWithPriority() {
	result := testutil.RunCLIExpectSuccessWithStdin(s.T(), s.workingDir, "Priority section content", "add", "section", "Priority Section", "--priority", "8")

	result.AssertStdoutContains(s.T(), "Added section")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "Priority Section")
	s.Contains(content, "Priority section content")
}

func (s *CRUDCLITestSuite) TestUpdateSection() {
	// First add a section
	testutil.RunCLIExpectSuccessWithStdin(s.T(), s.workingDir, "Original content", "add", "section", "Original Section")

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "section", "Original Section",
		"--content", "Updated content")

	result.AssertStdoutContains(s.T(), "Updated section")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "Original Section") // Name doesn't change
	s.Contains(content, "Updated content")
}

func (s *CRUDCLITestSuite) TestDeleteSection() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "section", "Development Guidelines")

	result.AssertStdoutContains(s.T(), "Deleted section")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.NotContains(content, "Development Guidelines")
}

// ========== Agent CRUD Tests ==========

func (s *CRUDCLITestSuite) TestAddAgent() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "agent", "test-agent",
		"--description", "Test agent description",
		"--priority", "8",
		"--tools", "Read,Edit,Grep",
		"--system-prompt", "You are a test agent")

	result.AssertStdoutContains(s.T(), "Added agent")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "test-agent")
	s.Contains(content, "Test agent description")
	s.Contains(content, "Read")
	s.Contains(content, "Edit")
}

func (s *CRUDCLITestSuite) TestAddAgentWithSystemPrompt() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "agent", "prompt-agent",
		"--description", "Agent with system prompt",
		"--system-prompt", "You are a helpful assistant")

	result.AssertStdoutContains(s.T(), "Added agent")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "prompt-agent")
	s.Contains(content, "You are a helpful assistant")
}

func (s *CRUDCLITestSuite) TestUpdateAgent() {
	// First add an agent
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "agent", "original-agent",
		"--description", "Original description",
		"--system-prompt", "Original prompt")

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "agent", "original-agent",
		"--description", "Updated description",
		"--priority", "9")

	result.AssertStdoutContains(s.T(), "Updated agent")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "Updated description")
}

func (s *CRUDCLITestSuite) TestDeleteAgent() {
	// First add an agent
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "agent", "delete-me",
		"--description", "This will be deleted",
		"--system-prompt", "Delete me")

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "agent", "delete-me")

	result.AssertStdoutContains(s.T(), "Deleted agent")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.NotContains(content, "delete-me")
}

// ========== Output CRUD Tests ==========

func (s *CRUDCLITestSuite) TestAddOutput() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "output", "NEW_OUTPUT.md")

	result.AssertStdoutContains(s.T(), "Added output")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "NEW_OUTPUT.md")
}

func (s *CRUDCLITestSuite) TestAddOutputWithType() {
	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "output", "TYPED_OUTPUT.md",
		"--type", "rule")

	result.AssertStdoutContains(s.T(), "Added output")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "TYPED_OUTPUT.md")
	s.Contains(content, "type: rule")
}

func (s *CRUDCLITestSuite) TestUpdateOutput() {
	// First add an output so we can update it
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "output", "TEST.md")

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "output", "TEST.md",
		"--type", "both")

	result.AssertStdoutContains(s.T(), "Updated output")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "TEST.md")
	s.Contains(content, "type: both")
}

func (s *CRUDCLITestSuite) TestDeleteOutput() {
	// First add an output so we can delete it
	testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "output", "DELETE_ME.md")

	result := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "output", "DELETE_ME.md")

	result.AssertStdoutContains(s.T(), "Deleted output")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	content := testutil.ReadFile(s.T(), configPath)
	s.NotContains(content, "DELETE_ME.md")
}

// ========== Error Cases ==========

func (s *CRUDCLITestSuite) TestCRUDWithoutConfig() {
	// Try to add a rule without config in empty directory
	emptyDir := testutil.CreateTempDir(s.T())
	result := testutil.RunCLIExpectError(s.T(), emptyDir, "add", "rule", "--name", "Test", "--content", "Test")

	result.AssertStderrContains(s.T(), "configuration file")
}

func (s *CRUDCLITestSuite) TestAddRuleMissingContent() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "add", "rule", "--name", "Test Rule", "--priority", "5")

	result.AssertStderrContains(s.T(), "required")
}

func (s *CRUDCLITestSuite) TestAddSectionMissingTitle() {
	// Section command requires a title argument, test with no arguments
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "add", "section")

	result.AssertStderrContains(s.T(), "accepts 1 arg(s), received 0")
}

func (s *CRUDCLITestSuite) TestAddAgentMissingName() {
	// Agent command requires a name argument, test with no arguments
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "add", "agent")

	result.AssertStderrContains(s.T(), "accepts 1 arg(s), received 0")
}

func (s *CRUDCLITestSuite) TestAddOutputMissingPath() {
	// Output command requires a filename argument, test with no arguments
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "add", "output")

	result.AssertStderrContains(s.T(), "accepts 1 arg(s), received 0")
}

func (s *CRUDCLITestSuite) TestUpdateNonExistentRule() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "update", "rule", "--name", "NonExistent Rule", "--content", "Test")

	result.AssertStdoutContains(s.T(), "not found")
}

func (s *CRUDCLITestSuite) TestUpdateRuleMissingName() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "update", "rule", "--content", "Test")

	result.AssertStderrContains(s.T(), "required")
}
