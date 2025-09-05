package cli

import (
	"testing"

	"github.com/Goldziher/ai-rulez/testing/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type CRUDCLITestSuite struct {
	testutil.BaseE2ESuite
}

func TestCRUDCLISuite(t *testing.T) {
	suite.Run(t, new(CRUDCLITestSuite))
}

func (s *CRUDCLITestSuite) TestRuleCRUD_FullCycle() {
	addResult := s.RunCLIExpectSuccess("add", "rule", "Test Rule",
		"--id", "test-rule-1",
		"--content", "New test rule content",
		"--priority", "high",
		"--target", "*.md")
	addResult.AssertStdoutContains(s.T(), "Added rule")

	getResult := s.RunCLIExpectSuccess("get", "rule", "Test Rule")
	getResult.AssertStdoutContains(s.T(), "Name:     Test Rule")
	getResult.AssertStdoutContains(s.T(), "ID:       test-rule-1")
	getResult.AssertStdoutContains(s.T(), "Content:  New test rule content")
	getResult.AssertStdoutContains(s.T(), "Priority: high")
	getResult.AssertStdoutContains(s.T(), "Targets:  [*.md]")

	listResult := s.RunCLIExpectSuccess("list", "rules")
	listResult.AssertStdoutContains(s.T(), "Test Rule")

	updateResult := s.RunCLIExpectSuccess("update", "rule", "Test Rule",
		"--content", "Updated content")
	updateResult.AssertStdoutContains(s.T(), "Updated rule")

	deleteResult := s.RunCLIExpectSuccess("delete", "rule", "Test Rule")
	deleteResult.AssertStdoutContains(s.T(), "Deleted rule")
}

func (s *CRUDCLITestSuite) TestSectionCRUD_FullCycle() {
	addResult := s.RunCLIExpectSuccess("add", "section", "New Section",
		"--id", "new-section-1",
		"--content", "New section content",
		"--priority", "high",
		"--target", "docs/*")
	addResult.AssertStdoutContains(s.T(), "Added section")

	getResult := s.RunCLIExpectSuccess("get", "section", "New Section")
	getResult.AssertStdoutContains(s.T(), "Name:     New Section")
	getResult.AssertStdoutContains(s.T(), "ID:       new-section-1")
	getResult.AssertStdoutContains(s.T(), "Content:  New section content")
	getResult.AssertStdoutContains(s.T(), "Priority: high")
	getResult.AssertStdoutContains(s.T(), "Targets:  [docs/*]")

	listResult := s.RunCLIExpectSuccess("list", "sections")
	listResult.AssertStdoutContains(s.T(), "New Section")

	updateResult := s.RunCLIExpectSuccess("update", "section", "New Section",
		"--content", "Updated content")
	updateResult.AssertStdoutContains(s.T(), "Updated section")

	deleteResult := s.RunCLIExpectSuccess("delete", "section", "New Section")
	deleteResult.AssertStdoutContains(s.T(), "Deleted section")
}

func (s *CRUDCLITestSuite) TestAgentCRUD_FullCycle() {
	addResult := s.RunCLIExpectSuccess("add", "agent", "test-agent",
		"--id", "test-agent-1",
		"--description", "Test agent description",
		"--system-prompt", "You are a test agent",
		"--tools", "Read,Grep")
	addResult.AssertStdoutContains(s.T(), "Added agent")

	getResult := s.RunCLIExpectSuccess("get", "agent", "test-agent")
	getResult.AssertStdoutContains(s.T(), "Name:         test-agent")
	getResult.AssertStdoutContains(s.T(), "ID:           test-agent-1")
	getResult.AssertStdoutContains(s.T(), "Description:   Test agent description")
	getResult.AssertStdoutContains(s.T(), "Priority:     medium")
	getResult.AssertStdoutContains(s.T(), "System Prompt: You are a test agent")
	getResult.AssertStdoutContains(s.T(), "Tools:        [Read Grep]")

	listResult := s.RunCLIExpectSuccess("list", "agents")
	listResult.AssertStdoutContains(s.T(), "test-agent")

	updateResult := s.RunCLIExpectSuccess("update", "agent", "test-agent",
		"--description", "Updated description",
		"--system-prompt", "You are an updated test agent")
	updateResult.AssertStdoutContains(s.T(), "Updated agent")

	getUpdatedResult := s.RunCLIExpectSuccess("get", "agent", "test-agent")
	getUpdatedResult.AssertStdoutContains(s.T(), "Description:   Updated description")
	getUpdatedResult.AssertStdoutContains(s.T(), "System Prompt: You are an updated test agent")

	deleteResult := s.RunCLIExpectSuccess("delete", "agent", "test-agent")
	deleteResult.AssertStdoutContains(s.T(), "Deleted agent")
}

func (s *CRUDCLITestSuite) TestOutputCRUD_FullCycle() {
	addResult := s.RunCLIExpectSuccess("add", "output", "NEW_OUTPUT",
		"--path", "NEW_OUTPUT.md",
		"--naming-scheme", "{name}.txt",
		"--type", "rule")
	addResult.AssertStdoutContains(s.T(), "Added output")

	getResult := s.RunCLIExpectSuccess("get", "output", "NEW_OUTPUT.md")
	getResult.AssertStdoutContains(s.T(), "Path:         NEW_OUTPUT.md")
	getResult.AssertStdoutContains(s.T(), "Type:         rule")
	getResult.AssertStdoutContains(s.T(), "Naming Scheme: {name}.txt")

	listResult := s.RunCLIExpectSuccess("list", "outputs")
	listResult.AssertStdoutContains(s.T(), "NEW_OUTPUT.md")

	updateResult := s.RunCLIExpectSuccess("update", "output", "NEW_OUTPUT.md",
		"--naming-scheme", "{name}_v2.txt")
	updateResult.AssertStdoutContains(s.T(), "Updated output")

	deleteResult := s.RunCLIExpectSuccess("delete", "output", "NEW_OUTPUT.md")
	deleteResult.AssertStdoutContains(s.T(), "Deleted output")
}

func (s *CRUDCLITestSuite) TestMCPServerCRUD_FullCycle() {
	addResult := s.RunCLIExpectSuccess("add", "mcp-server", "test-server",
		"--description", "Test Server",
		"--command", "npx",
		"--args", "-y",
		"--args", "@test/server",
		"--transport", "stdio")
	addResult.AssertStdoutContains(s.T(), "Added MCP server")

	getResult := s.RunCLIExpectSuccess("get", "mcp-server", "test-server")
	getResult.AssertStdoutContains(s.T(), "Name:         test-server")
	getResult.AssertStdoutContains(s.T(), "Description:  Test Server")
	getResult.AssertStdoutContains(s.T(), "Command:      npx")
	getResult.AssertStdoutContains(s.T(), "Args:         [-y @test/server]")
	getResult.AssertStdoutContains(s.T(), "Transport:    stdio")

	listResult := s.RunCLIExpectSuccess("list", "mcp-servers")
	listResult.AssertStdoutContains(s.T(), "test-server")

	updateResult := s.RunCLIExpectSuccess("update", "mcp-server", "test-server",
		"--description", "Updated Description",
		"--transport", "http",
		"--url", "http://localhost:8080")
	updateResult.AssertStdoutContains(s.T(), "Updated MCP server")

	getUpdatedResult := s.RunCLIExpectSuccess("get", "mcp-server", "test-server")
	getUpdatedResult.AssertStdoutContains(s.T(), "Description:  Updated Description")
	getUpdatedResult.AssertStdoutContains(s.T(), "Transport:    http")
	getUpdatedResult.AssertStdoutContains(s.T(), "URL:          http://localhost:8080")

	deleteResult := s.RunCLIExpectSuccess("delete", "mcp-server", "test-server")
	deleteResult.AssertStdoutContains(s.T(), "Deleted MCP server")
}

func (s *CRUDCLITestSuite) TestCommandCRUD_FullCycle() {
	addResult := s.RunCLIExpectSuccess("add", "command", "test-cmd",
		"--description", "Test Command",
		"--command", "echo 'test command'",
		"--args", "arg1",
		"--args", "arg2")
	addResult.AssertStdoutContains(s.T(), "Added command")

	getResult := s.RunCLIExpectSuccess("get", "command", "test-cmd")
	getResult.AssertStdoutContains(s.T(), "Name:         test-cmd")
	getResult.AssertStdoutContains(s.T(), "Description:  Test Command")

	listResult := s.RunCLIExpectSuccess("list", "commands")
	listResult.AssertStdoutContains(s.T(), "test-cmd")

	updateResult := s.RunCLIExpectSuccess("update", "command", "test-cmd",
		"--description", "Updated Description")
	updateResult.AssertStdoutContains(s.T(), "Updated command")

	deleteResult := s.RunCLIExpectSuccess("delete", "command", "test-cmd")
	deleteResult.AssertStdoutContains(s.T(), "Deleted command")
}

func (s *CRUDCLITestSuite) TestMetadataCRUD_FullCycle() {
	getResult := s.RunCLIExpectSuccess("get", "metadata")
	getResult.AssertStdoutContains(s.T(), "Name:        Test Project")

	setResult := s.RunCLIExpectSuccess("set", "metadata",
		"--name", "Updated Project",
		"--version", "2.0.0",
		"--description", "Updated description")
	setResult.AssertStdoutContains(s.T(), "Updated metadata")

	getUpdatedResult := s.RunCLIExpectSuccess("get", "metadata")
	getUpdatedResult.AssertStdoutContains(s.T(), "Name:        Updated Project")
	getUpdatedResult.AssertStdoutContains(s.T(), "Version:     2.0.0")
	getUpdatedResult.AssertStdoutContains(s.T(), "Description: Updated description")
}

func (s *CRUDCLITestSuite) TestExtendsCRUD_FullCycle() {
	s.WriteConfigFile("shared.yaml", `
metadata:
  name: Shared Config
  version: 1.0.0
rules: []
outputs: []
`)

	getResult := s.RunCLIExpectSuccess("get", "extends")
	getResult.AssertStdoutContains(s.T(), "not set")

	setResult := s.RunCLIExpectSuccess("set", "extends", "./shared.yaml")
	setResult.AssertStdoutContains(s.T(), "Updated extends")

	getUpdatedResult := s.RunCLIExpectSuccess("get", "extends")
	getUpdatedResult.AssertStdoutContains(s.T(), "./shared.yaml")

	deleteResult := s.RunCLIExpectSuccess("delete", "extends")
	deleteResult.AssertStdoutContains(s.T(), "Deleted extends")

	getFinalResult := s.RunCLIExpectSuccess("get", "extends")
	getFinalResult.AssertStdoutContains(s.T(), "not set")
}

func (s *CRUDCLITestSuite) TestIncludesCRUD_FullCycle() {
	s.WriteConfigFile("common.yaml", `
rules:
  - name: Common Rule
    content: Common content
`)

	listResult := s.RunCLIExpectSuccess("list", "includes")
	listResult.AssertStdoutContains(s.T(), "No includes found")

	addResult1 := s.RunCLIExpectSuccess("add", "include", "./common.yaml")
	addResult1.AssertStdoutContains(s.T(), "Added include")

	addResult2 := s.RunCLIExpectSuccess("add", "include", "https://example.com/rules.yaml")
	addResult2.AssertStdoutContains(s.T(), "Added include")

	listUpdatedResult := s.RunCLIExpectSuccess("list", "includes")
	listUpdatedResult.AssertStdoutContains(s.T(), "./common.yaml")
	listUpdatedResult.AssertStdoutContains(s.T(), "https://example.com/rules.yaml")

	deleteResult := s.RunCLIExpectSuccess("delete", "include", "./common.yaml")
	deleteResult.AssertStdoutContains(s.T(), "Deleted include")

	listFinalResult := s.RunCLIExpectSuccess("list", "includes")
	s.NotContains(listFinalResult.Stdout, "./common.yaml")
	listFinalResult.AssertStdoutContains(s.T(), "https://example.com/rules.yaml")
}

func (s *CRUDCLITestSuite) TestCRUDWithoutConfig() {
	emptyDir := testutil.CreateTempDir(s.T())
	result := testutil.RunCLIExpectError(s.T(), emptyDir, "add", "rule", "Test Rule", "--content", "Test")

	result.AssertStderrContains(s.T(), "configuration file")
}

func (s *CRUDCLITestSuite) TestAddRuleMissingContent() {
	result := s.RunCLIExpectError("add", "rule", "Test Rule")

	result.AssertStderrContains(s.T(), "required flag(s) \"content\" not set")
}

func (s *CRUDCLITestSuite) TestAddSectionMissingTitle() {
	result := s.RunCLIExpectError("add", "section")

	result.AssertStderrContains(s.T(), "accepts 1 arg(s), received 0")
}

func (s *CRUDCLITestSuite) TestAddAgentMissingName() {
	result := s.RunCLIExpectError("add", "agent")

	result.AssertStderrContains(s.T(), "accepts 1 arg(s), received 0")
}

func (s *CRUDCLITestSuite) TestAddOutputMissingPath() {
	result := s.RunCLIExpectError("add", "output")

	result.AssertStderrContains(s.T(), "accepts 1 arg(s), received 0")
}

func (s *CRUDCLITestSuite) TestUpdateNonExistentRule() {
	result := s.RunCLIExpectError("update", "rule", "NonExistent Rule", "--content", "Test")

	result.AssertStderrContains(s.T(), "not found")
}

func (s *CRUDCLITestSuite) TestUpdateRuleMissingName() {
	result := s.RunCLIExpectError("update", "rule", "--content", "Test")

	result.AssertStderrContains(s.T(), "accepts 1 arg(s), received 0")
}
