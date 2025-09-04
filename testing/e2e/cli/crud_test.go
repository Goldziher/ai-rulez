package cli

import (
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

func (s *CRUDCLITestSuite) TestRuleCRUD_FullCycle() {
	addResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "rule", "Test Rule",
		"--id", "test-rule-1",
		"--content", "New test rule content",
		"--priority", "high",
		"--target", "*.md")
	addResult.AssertStdoutContains(s.T(), "Added rule")

	getResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "rule", "Test Rule")
	getResult.AssertStdoutContains(s.T(), "Name:     Test Rule")
	getResult.AssertStdoutContains(s.T(), "ID:       test-rule-1")
	getResult.AssertStdoutContains(s.T(), "Content:  New test rule content")
	getResult.AssertStdoutContains(s.T(), "Priority: high")
	getResult.AssertStdoutContains(s.T(), "Targets:  [*.md]")

	listResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "rules")
	listResult.AssertStdoutContains(s.T(), "Test Rule")

	updateResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "rule", "Test Rule",
		"--content", "Updated content")
	updateResult.AssertStdoutContains(s.T(), "Updated rule")

	deleteResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "rule", "Test Rule")
	deleteResult.AssertStdoutContains(s.T(), "Deleted rule")
}

func (s *CRUDCLITestSuite) TestSectionCRUD_FullCycle() {
	addResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "section", "New Section",
		"--id", "new-section-1",
		"--content", "New section content",
		"--priority", "high",
		"--target", "docs/*")
	addResult.AssertStdoutContains(s.T(), "Added section")

	getResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "section", "New Section")
	getResult.AssertStdoutContains(s.T(), "Name:     New Section")
	getResult.AssertStdoutContains(s.T(), "ID:       new-section-1")
	getResult.AssertStdoutContains(s.T(), "Content:  New section content")
	getResult.AssertStdoutContains(s.T(), "Priority: high")
	getResult.AssertStdoutContains(s.T(), "Targets:  [docs/*]")

	listResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "sections")
	listResult.AssertStdoutContains(s.T(), "New Section")

	updateResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "section", "New Section",
		"--content", "Updated content")
	updateResult.AssertStdoutContains(s.T(), "Updated section")

	deleteResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "section", "New Section")
	deleteResult.AssertStdoutContains(s.T(), "Deleted section")
}

func (s *CRUDCLITestSuite) TestAgentCRUD_FullCycle() {
	addResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "agent", "test-agent",
		"--id", "test-agent-1",
		"--description", "Test agent description",
		"--system-prompt", "You are a test agent",
		"--tools", "Read,Grep")
	addResult.AssertStdoutContains(s.T(), "Added agent")

	getResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "agent", "test-agent")
	getResult.AssertStdoutContains(s.T(), "Name:         test-agent")
	getResult.AssertStdoutContains(s.T(), "ID:           test-agent-1")
	getResult.AssertStdoutContains(s.T(), "Description:   Test agent description")
	getResult.AssertStdoutContains(s.T(), "Priority:     medium")
	getResult.AssertStdoutContains(s.T(), "System Prompt: You are a test agent")
	getResult.AssertStdoutContains(s.T(), "Tools:        [Read Grep]")

	listResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "agents")
	listResult.AssertStdoutContains(s.T(), "test-agent")

	updateResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "agent", "test-agent",
		"--description", "Updated description",
		"--template-type", "inline",
		"--template-value", "New prompt")
	updateResult.AssertStdoutContains(s.T(), "Updated agent")

	getUpdatedResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "agent", "test-agent")
	getUpdatedResult.AssertStdoutContains(s.T(), "Description:   Updated description")

	deleteResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "agent", "test-agent")
	deleteResult.AssertStdoutContains(s.T(), "Deleted agent")
}

func (s *CRUDCLITestSuite) TestOutputCRUD_FullCycle() {
	addResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "output", "NEW_OUTPUT.md",
		"--naming-scheme", "{name}.txt",
		"--template-type", "inline",
		"--template-value", "Hello {{.ProjectName}}")
	addResult.AssertStdoutContains(s.T(), "Added output")

	getResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "output", "NEW_OUTPUT.md")
	getResult.AssertStdoutContains(s.T(), "Path:         NEW_OUTPUT.md")
	getResult.AssertStdoutContains(s.T(), "Naming Scheme: {name}.txt")
	getResult.AssertStdoutContains(s.T(), "Template Type: inline")
	getResult.AssertStdoutContains(s.T(), "Template Value: Hello {{.ProjectName}}")

	listResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "outputs")
	listResult.AssertStdoutContains(s.T(), "NEW_OUTPUT.md")

	updateResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "output", "NEW_OUTPUT.md",
		"--naming-scheme", "{name}_v2.txt")
	updateResult.AssertStdoutContains(s.T(), "Updated output")

	deleteResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "output", "NEW_OUTPUT.md")
	deleteResult.AssertStdoutContains(s.T(), "Deleted output")
}

func (s *CRUDCLITestSuite) TestMCPServerCRUD_FullCycle() {
	addResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "mcp-server", "test-server",
		"--description", "Test Server",
		"--command", "npx",
		"--arg", "-y",
		"--arg", "@test/server",
		"--env", "TOKEN=123",
		"--transport", "stdio")
	addResult.AssertStdoutContains(s.T(), "Added MCP server")

	getResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "mcp-server", "test-server")
	getResult.AssertStdoutContains(s.T(), "Name:         test-server")
	getResult.AssertStdoutContains(s.T(), "Description:  Test Server")
	getResult.AssertStdoutContains(s.T(), "Command:      npx")
	getResult.AssertStdoutContains(s.T(), "Args:         [-y @test/server]")
	getResult.AssertStdoutContains(s.T(), "Env:          [TOKEN=123]")

	listResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "mcp-servers")
	listResult.AssertStdoutContains(s.T(), "test-server")

	updateResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "mcp-server", "test-server",
		"--description", "Updated Description",
		"--transport", "http",
		"--url", "http://localhost:8080")
	updateResult.AssertStdoutContains(s.T(), "Updated MCP server")

	getUpdatedResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "mcp-server", "test-server")
	getUpdatedResult.AssertStdoutContains(s.T(), "Description:  Updated Description")
	getUpdatedResult.AssertStdoutContains(s.T(), "Transport:    http")
	getUpdatedResult.AssertStdoutContains(s.T(), "URL:          http://localhost:8080")

	deleteResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "mcp-server", "test-server")
	deleteResult.AssertStdoutContains(s.T(), "Deleted MCP server")
}

func (s *CRUDCLITestSuite) TestCommandCRUD_FullCycle() {
	addResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "command", "test-cmd",
		"--description", "Test Command",
		"--alias", "tcmd",
		"--usage", "/test-cmd <arg>",
		"--system-prompt", "You are a test command.",
		"--shortcut", "Ctrl+T")
	addResult.AssertStdoutContains(s.T(), "Added command")

	getResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "command", "test-cmd")
	getResult.AssertStdoutContains(s.T(), "Name:         test-cmd")
	getResult.AssertStdoutContains(s.T(), "Description:  Test Command")
	getResult.AssertStdoutContains(s.T(), "Aliases:      [tcmd]")
	getResult.AssertStdoutContains(s.T(), "Usage:        /test-cmd <arg>")

	listResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "commands")
	listResult.AssertStdoutContains(s.T(), "test-cmd")

	updateResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "update", "command", "test-cmd",
		"--description", "Updated Description")
	updateResult.AssertStdoutContains(s.T(), "Updated command")

	deleteResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "command", "test-cmd")
	deleteResult.AssertStdoutContains(s.T(), "Deleted command")
}

func (s *CRUDCLITestSuite) TestMetadataCRUD_FullCycle() {
	getResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "metadata")
	getResult.AssertStdoutContains(s.T(), "Name:        Test Project")

	setResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "set", "metadata",
		"--name", "Updated Project",
		"--version", "2.0.0",
		"--description", "Updated description")
	setResult.AssertStdoutContains(s.T(), "Updated metadata")

	getUpdatedResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "metadata")
	getUpdatedResult.AssertStdoutContains(s.T(), "Name:        Updated Project")
	getUpdatedResult.AssertStdoutContains(s.T(), "Version:     2.0.0")
	getUpdatedResult.AssertStdoutContains(s.T(), "Description: Updated description")
}

func (s *CRUDCLITestSuite) TestExtendsCRUD_FullCycle() {
	testutil.WriteFile(s.T(), s.workingDir, "shared.yaml", `
metadata:
  name: Shared Config
  version: 1.0.0
rules: []
outputs: []
`)

	getResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "extends")
	getResult.AssertStdoutContains(s.T(), "not set")

	setResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "set", "extends", "./shared.yaml")
	setResult.AssertStdoutContains(s.T(), "Updated extends")

	getUpdatedResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "extends")
	getUpdatedResult.AssertStdoutContains(s.T(), "./shared.yaml")

	deleteResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "extends")
	deleteResult.AssertStdoutContains(s.T(), "Deleted extends")

	getFinalResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "get", "extends")
	getFinalResult.AssertStdoutContains(s.T(), "not set")
}

func (s *CRUDCLITestSuite) TestIncludesCRUD_FullCycle() {
	testutil.WriteFile(s.T(), s.workingDir, "common.yaml", `
rules:
  - name: Common Rule
    content: Common content
`)

	listResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "includes")
	listResult.AssertStdoutContains(s.T(), "No includes found")

	addResult1 := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "include", "./common.yaml")
	addResult1.AssertStdoutContains(s.T(), "Added include")

	addResult2 := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "add", "include", "https://example.com/rules.yaml")
	addResult2.AssertStdoutContains(s.T(), "Added include")

	listUpdatedResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "includes")
	listUpdatedResult.AssertStdoutContains(s.T(), "./common.yaml")
	listUpdatedResult.AssertStdoutContains(s.T(), "https://example.com/rules.yaml")

	deleteResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "delete", "include", "./common.yaml")
	deleteResult.AssertStdoutContains(s.T(), "Deleted include")

	listFinalResult := testutil.RunCLIExpectSuccess(s.T(), s.workingDir, "list", "includes")
	s.NotContains(listFinalResult.Stdout, "./common.yaml")
	listFinalResult.AssertStdoutContains(s.T(), "https://example.com/rules.yaml")
}

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

	result.AssertStderrContains(s.T(), "not found")
}

func (s *CRUDCLITestSuite) TestUpdateRuleMissingName() {
	result := testutil.RunCLIExpectError(s.T(), s.workingDir, "update", "rule", "--content", "Test")

	result.AssertStderrContains(s.T(), "accepts 1 arg(s), received 0")
}
