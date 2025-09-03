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
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)
}

func (s *CRUDCLITestSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

// ========== Rule CRUD Tests ==========

func (s *CRUDCLITestSuite) TestRuleCRUD_FullCycle() {
	// ADD
	addResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule", "Test Rule",
		"--id", "test-rule-1",
		"--content", "New test rule content",
		"--priority", "high",
		"--target", "*.md")
	addResult.AssertStdoutContains(s.T(), "Added rule")

	// GET
	getResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "rule", "Test Rule")
	getResult.AssertStdoutContains(s.T(), "Name:     Test Rule")
	getResult.AssertStdoutContains(s.T(), "ID:       test-rule-1")
	getResult.AssertStdoutContains(s.T(), "Content:  New test rule content")
	getResult.AssertStdoutContains(s.T(), "Priority: high")
	getResult.AssertStdoutContains(s.T(), "Targets:  [*.md]")

	// LIST
	listResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "rules")
	listResult.AssertStdoutContains(s.T(), "Test Rule")

	// UPDATE
	updateResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "rule", "Test Rule",
		"--content", "Updated content")
	updateResult.AssertStdoutContains(s.T(), "Updated rule")

	// DELETE
	deleteResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "rule", "Test Rule")
	deleteResult.AssertStdoutContains(s.T(), "Deleted rule")
}

// ========== Section CRUD Tests ==========

func (s *CRUDCLITestSuite) TestSectionCRUD_FullCycle() {
	// ADD
	addResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "section", "New Section",
		"--id", "new-section-1",
		"--content", "New section content",
		"--priority", "high",
		"--target", "docs/*")
	addResult.AssertStdoutContains(s.T(), "Added section")

	// GET
	getResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "section", "New Section")
	getResult.AssertStdoutContains(s.T(), "Name:     New Section")
	getResult.AssertStdoutContains(s.T(), "ID:       new-section-1")
	getResult.AssertStdoutContains(s.T(), "Content:  New section content")
	getResult.AssertStdoutContains(s.T(), "Priority: high")
	getResult.AssertStdoutContains(s.T(), "Targets:  [docs/*]")

	// LIST
	listResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "sections")
	listResult.AssertStdoutContains(s.T(), "New Section")

	// UPDATE
	updateResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "section", "New Section",
		"--content", "Updated content")
	updateResult.AssertStdoutContains(s.T(), "Updated section")

	// DELETE
	deleteResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "section", "New Section")
	deleteResult.AssertStdoutContains(s.T(), "Deleted section")
}

// ========== Agent CRUD Tests ==========

func (s *CRUDCLITestSuite) TestAgentCRUD_FullCycle() {
	// ADD
	addResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "agent", "test-agent",
		"--id", "test-agent-1",
		"--description", "Test agent description",
		"--system-prompt", "You are a test agent",
		"--tools", "Read,Grep")
	addResult.AssertStdoutContains(s.T(), "Added agent")

	// GET
	getResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "agent", "test-agent")
	getResult.AssertStdoutContains(s.T(), "Name:         test-agent")
	getResult.AssertStdoutContains(s.T(), "ID:           test-agent-1")
	getResult.AssertStdoutContains(s.T(), "System Prompt: You are a test agent")
	getResult.AssertStdoutContains(s.T(), "Tools:        [Read Grep]")

	// LIST
	listResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "agents")
	listResult.AssertStdoutContains(s.T(), "test-agent")

	// UPDATE with template
	updateResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "agent", "test-agent",
		"--description", "Updated description",
		"--template-type", "inline",
		"--template-value", "New prompt")
	updateResult.AssertStdoutContains(s.T(), "Updated agent")

	// GET after update
	getUpdatedResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "agent", "test-agent")
	getUpdatedResult.AssertStdoutContains(s.T(), "Description:   Updated description")
	getUpdatedResult.AssertStdoutContains(s.T(), "Template Type: inline")
	getUpdatedResult.AssertStdoutContains(s.T(), "Template Value: New prompt")

	// DELETE
	deleteResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "agent", "test-agent")
	deleteResult.AssertStdoutContains(s.T(), "Deleted agent")
}

// ========== Output CRUD Tests ==========

func (s *CRUDCLITestSuite) TestOutputCRUD_FullCycle() {
	// ADD
	addResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "output", "NEW_OUTPUT.md",
		"--naming-scheme", "{name}.txt",
		"--template-type", "inline",
		"--template-value", "Hello {{.ProjectName}}")
	addResult.AssertStdoutContains(s.T(), "Added output")

	// GET
	getResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "output", "NEW_OUTPUT.md")
	getResult.AssertStdoutContains(s.T(), "Path:          NEW_OUTPUT.md")
	getResult.AssertStdoutContains(s.T(), "Naming Scheme: {name}.txt")
	getResult.AssertStdoutContains(s.T(), "Template Type: inline")
	getResult.AssertStdoutContains(s.T(), "Template Value: Hello {{.ProjectName}}")

	// LIST
	listResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "outputs")
	listResult.AssertStdoutContains(s.T(), "NEW_OUTPUT.md")

	// UPDATE
	updateResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "output", "NEW_OUTPUT.md",
		"--naming-scheme", "{name}_v2.txt")
	updateResult.AssertStdoutContains(s.T(), "Updated output")

	// DELETE
	deleteResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "output", "NEW_OUTPUT.md")
	deleteResult.AssertStdoutContains(s.T(), "Deleted output")
}

// ========== MCP Server CRUD Tests ==========

func (s *CRUDCLITestSuite) TestMCPServerCRUD_FullCycle() {
	// ADD
	addResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "mcp-server", "test-server",
		"--description", "Test Server",
		"--command", "npx",
		"--arg", "-y",
		"--arg", "@test/server",
		"--env", "TOKEN=123",
		"--transport", "stdio")
	addResult.AssertStdoutContains(s.T(), "Added mcp_server")

	// GET
	getResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "mcp-server", "test-server")
	getResult.AssertStdoutContains(s.T(), "Name:        test-server")
	getResult.AssertStdoutContains(s.T(), "Command:     npx")
	getResult.AssertStdoutContains(s.T(), "Args:        [-y @test/server]")
	getResult.AssertStdoutContains(s.T(), "Env:         map[TOKEN:123]")

	// LIST
	listResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "mcp-servers")
	listResult.AssertStdoutContains(s.T(), "test-server")

	// UPDATE
	updateResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "mcp-server", "test-server",
		"--description", "Updated Description",
		"--transport", "http",
		"--url", "http://localhost:8080")
	updateResult.AssertStdoutContains(s.T(), "Updated mcp_server")

	// GET after update
	getUpdatedResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "mcp-server", "test-server")
	getUpdatedResult.AssertStdoutContains(s.T(), "Description: Updated Description")
	getUpdatedResult.AssertStdoutContains(s.T(), "Transport:   http")
	getUpdatedResult.AssertStdoutContains(s.T(), "URL:         http://localhost:8080")

	// DELETE
	deleteResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "mcp-server", "test-server")
	deleteResult.AssertStdoutContains(s.T(), "Deleted mcp_server")
}

// ========== Command CRUD Tests ==========

func (s *CRUDCLITestSuite) TestCommandCRUD_FullCycle() {
	// ADD
	addResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "command", "test-cmd",
		"--description", "Test Command",
		"--alias", "tcmd",
		"--usage", "/test-cmd <arg>",
		"--system-prompt", "You are a test command.",
		"--shortcut", "Ctrl+T")
	addResult.AssertStdoutContains(s.T(), "Added command")

	// GET
	getResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "command", "test-cmd")
	getResult.AssertStdoutContains(s.T(), "Name:        test-cmd")
	getResult.AssertStdoutContains(s.T(), "Aliases:     [tcmd]")
	getResult.AssertStdoutContains(s.T(), "Usage:       /test-cmd <arg>")

	// LIST
	listResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "commands")
	listResult.AssertStdoutContains(s.T(), "test-cmd")

	// UPDATE
	updateResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "command", "test-cmd",
		"--description", "Updated Description")
	updateResult.AssertStdoutContains(s.T(), "Updated command")

	// DELETE
	deleteResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "command", "test-cmd")
	deleteResult.AssertStdoutContains(s.T(), "Deleted command")
}

// ========== Singleton Property CRUD Tests ==========

func (s *CRUDCLITestSuite) TestMetadataCRUD_FullCycle() {
	// GET initial
	getResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "metadata")
	getResult.AssertStdoutContains(s.T(), "Name:    Test Project")

	// SET
	setResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "set", "metadata",
		"--name", "Updated Project",
		"--version", "2.0.0",
		"--description", "Updated description")
	setResult.AssertStdoutContains(s.T(), "Updated metadata")

	// GET updated
	getUpdatedResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "metadata")
	getUpdatedResult.AssertStdoutContains(s.T(), "Name:        Updated Project")
	getUpdatedResult.AssertStdoutContains(s.T(), "Version:     2.0.0")
	getUpdatedResult.AssertStdoutContains(s.T(), "Description: Updated description")
}

func (s *CRUDCLITestSuite) TestExtendsCRUD_FullCycle() {
	// GET initial (should be empty)
	getResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "extends")
	getResult.AssertStdoutContains(s.T(), "not set")

	// SET
	setResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "set", "extends", "./shared.yaml")
	setResult.AssertStdoutContains(s.T(), "Updated extends")

	// GET updated
	getUpdatedResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "extends")
	getUpdatedResult.AssertStdoutContains(s.T(), "./shared.yaml")

	// DELETE
	deleteResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "extends")
	deleteResult.AssertStdoutContains(s.T(), "Deleted extends")

	// GET final (should be empty again)
	getFinalResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "extends")
	getFinalResult.AssertStdoutContains(s.T(), "not set")
}

func (s *CRUDCLITestSuite) TestIncludesCRUD_FullCycle() {
	// LIST initial (should be empty)
	listResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "includes")
	listResult.AssertStdoutContains(s.T(), "No includes found")

	// ADD first include
	addResult1 := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "include", "./common.yaml")
	addResult1.AssertStdoutContains(s.T(), "Added include")

	// ADD second include
	addResult2 := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "include", "https://example.com/rules.yaml")
	addResult2.AssertStdoutContains(s.T(), "Added include")

	// LIST updated
	listUpdatedResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "includes")
	listUpdatedResult.AssertStdoutContains(s.T(), "./common.yaml")
	listUpdatedResult.AssertStdoutContains(s.T(), "https://example.com/rules.yaml")

	// DELETE
	deleteResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "include", "./common.yaml")
	deleteResult.AssertStdoutContains(s.T(), "Deleted include")

	// LIST final
	listFinalResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "includes")
	listFinalResult.AssertNotContains(s.T(), "./common.yaml")
	listFinalResult.AssertStdoutContains(s.T(), "https://example.com/rules.yaml")
}

// ========== Error Cases ==========

func (s *CRUDCLITestSuite) TestCRUDWithoutConfig() {
	emptyDir := testutil.CreateTempDir(s.T())
	result := testutil.RunCLIExpectError(s.T(), emptyDir, "add", "rule", "Test Rule", "--content", "Test")

	result.AssertStderrContains(s.T(), "configuration file")
}

func (s *CRUDCLITestSuite) TestAddRuleMissingContent() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "add", "rule", "Test Rule")

	result.AssertStderrContains(s.T(), "required flag(s) \"content\" not set")
}

func (s *CRUDCLITestSuite) TestAddSectionMissingTitle() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "add", "section")

	result.AssertStderrContains(s.T(), "accepts 1 arg(s), received 0")
}

func (s *CRUDCLITestSuite) TestAddAgentMissingName() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "add", "agent")

	result.AssertStderrContains(s.T(), "accepts 1 arg(s), received 0")
}

func (s *CRUDCLITestSuite) TestAddOutputMissingPath() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "add", "output")

	result.AssertStderrContains(s.T(), "accepts 1 arg(s), received 0")
}

func (s *CRUDCLITestSuite) TestUpdateNonExistentRule() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "update", "rule", "NonExistent Rule", "--content", "Test")

	result.AssertStdoutContains(s.T(), "not found")
}

func (s *CRUDCLITestSuite) TestUpdateRuleMissingName() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "update", "rule", "--content", "Test")

	result.AssertStderrContains(s.T(), "accepts 1 arg(s), received 0")
}
