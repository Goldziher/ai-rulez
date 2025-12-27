package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/tests/e2e/testutil"
	"github.com/stretchr/testify/suite"
)

type MCPServerE2ETestSuite struct {
	suite.Suite
	workingDir string
	client     *testutil.MCPClient
}

func TestMCPServerE2ESuite(t *testing.T) {
	suite.Run(t, new(MCPServerE2ETestSuite))
}

func (s *MCPServerE2ETestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)
	s.client = testutil.StartMCPServer(s.T(), s.workingDir)
}

func (s *MCPServerE2ETestSuite) TearDownTest() {
	if s.client != nil {
		s.client.Close()
	}
	testutil.CleanupTestBinary()
}

func (s *MCPServerE2ETestSuite) TestGetVersion() {
	response := s.client.CallTool(s.T(), "get_version", map[string]interface{}{})
	response.AssertToolSuccess(s.T())

	s.NotNil(response.Result, "Should have result")
	s.NotEmpty(response.Result.Content, "Should have content")
	s.Contains(response.Result.Content[0].Text, "version")
}

func (s *MCPServerE2ETestSuite) TestRuleCRUD_FullCycle() {
	// Add rule
	addParams := map[string]interface{}{
		"name":    "Test Rule",
		"content": "Test Content",
		"id":      "rule-1",
	}
	addResponse := s.client.CallTool(s.T(), "add_rule", addParams)
	addResponse.AssertToolSuccess(s.T())
	s.NotEmpty(addResponse.Result.Content)
	s.Contains(addResponse.Result.Content[0].Text, "Added rule")

	// Get rule
	getResponse := s.client.CallTool(s.T(), "get_rule", map[string]interface{}{"name": "Test Rule"})
	getResponse.AssertToolSuccess(s.T())
	s.NotEmpty(getResponse.Result.Content)
	s.Contains(getResponse.Result.Content[0].Text, "rule-1")
}

func (s *MCPServerE2ETestSuite) TestOutputCRUD_FullCycle() {
	// Add output
	addParams := map[string]interface{}{
		"path": "test.md",
		"type": "agent",
	}
	addResponse := s.client.CallTool(s.T(), "add_output", addParams)
	addResponse.AssertToolSuccess(s.T())
	s.NotEmpty(addResponse.Result.Content)
	s.Contains(addResponse.Result.Content[0].Text, "Added output")

	// Get output
	getResponse := s.client.CallTool(s.T(), "get_output", map[string]interface{}{"path": "test.md"})
	getResponse.AssertToolSuccess(s.T())
	s.NotEmpty(getResponse.Result.Content)
	s.Contains(getResponse.Result.Content[0].Text, "agent")
}

func (s *MCPServerE2ETestSuite) TestMCPServerCRUD_FullCycle() {
	// Add MCP server
	addParams := map[string]interface{}{
		"name":    "test-server",
		"command": "test",
		"id":      "server-1",
	}
	addResponse := s.client.CallTool(s.T(), "add_mcp_server", addParams)
	addResponse.AssertToolSuccess(s.T())
	s.NotEmpty(addResponse.Result.Content)
	s.Contains(addResponse.Result.Content[0].Text, "Added mcp_servers: test-server")

	// Get MCP server
	getResponse := s.client.CallTool(s.T(), "get_mcp_server", map[string]interface{}{"name": "test-server"})
	getResponse.AssertToolSuccess(s.T())
	s.NotEmpty(getResponse.Result.Content)
	s.Contains(getResponse.Result.Content[0].Text, "server-1")
}

func (s *MCPServerE2ETestSuite) TestCommandCRUD_FullCycle() {
	// Add command
	addParams := map[string]interface{}{
		"name":        "test-cmd",
		"description": "Test Desc",
		"id":          "cmd-1",
	}
	addResponse := s.client.CallTool(s.T(), "add_command", addParams)
	addResponse.AssertToolSuccess(s.T())
	s.NotEmpty(addResponse.Result.Content)
	s.Contains(addResponse.Result.Content[0].Text, "Added command")

	// Get command
	getResponse := s.client.CallTool(s.T(), "get_command", map[string]interface{}{"name": "test-cmd"})
	getResponse.AssertToolSuccess(s.T())
	s.NotEmpty(getResponse.Result.Content)
	s.Contains(getResponse.Result.Content[0].Text, "cmd-1")
}

func (s *MCPServerE2ETestSuite) TestInitProject() {
	// Remove existing config
	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	os.Remove(configPath)

	// Initialize project
	params := map[string]interface{}{
		"project_name": "MCP-Initialized-Project",
		"providers":    []string{"claude", "continue-dev"},
		"with_agents":  true,
	}
	response := s.client.CallTool(s.T(), "init_project", params)
	response.AssertToolSuccess(s.T())
	s.NotEmpty(response.Result.Content)
	s.Contains(response.Result.Content[0].Text, "initialized")

	// Verify config file exists
	s.True(testutil.FileExists(s.T(), configPath), "Config file should exist at %s", configPath)

	// Verify config content
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "MCP-Initialized-Project")
	s.Contains(content, "claude")
	s.Contains(content, "continue-dev")
}

func (s *MCPServerE2ETestSuite) TestGenerateAndValidate() {
	// Validate config
	validateResponse := s.client.CallTool(s.T(), "validate_config", map[string]interface{}{})
	validateResponse.AssertToolSuccess(s.T())
	s.NotEmpty(validateResponse.Result.Content)
	s.Contains(validateResponse.Result.Content[0].Text, "valid")

	// Generate outputs
	generateResponse := s.client.CallTool(s.T(), "generate_outputs", map[string]interface{}{})
	generateResponse.AssertToolSuccess(s.T())
	s.NotEmpty(generateResponse.Result.Content)
	s.Contains(generateResponse.Result.Content[0].Text, "generated")
}
