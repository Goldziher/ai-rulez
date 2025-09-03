package e2e

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/mcp"
	"github.com/Goldziher/ai-rulez/testing/e2e/testutil"
	"github.com/stretchr/testify/suite"
	"github.com/ybbus/jsonrpc/v3"
)

type MCPServerE2ETestSuite struct {
	suite.Suite
	workingDir string
	server     *mcp.Server
	client     jsonrpc.RPCClient
	lis        net.Listener
}

func TestMCPServerE2ESuite(t *testing.T) {
	suite.Run(t, new(MCPServerE2ETestSuite))
}

func (s *MCPServerE2ETestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)

	// Start the MCP server in-process
	s.server = mcp.NewServer("test")
	lis, err := net.Listen("tcp", ":0") // Use a random free port
	s.Require().NoError(err)
	s.lis = lis

	go func() {
		_ = s.server.GetMCPServer().Start(s.lis)
	}()

	// Create a JSON-RPC client to connect to the server
	conn, err := net.Dial("tcp", s.lis.Addr().String())
	s.Require().NoError(err)
	s.client = jsonrpc.NewClient(conn)
}

func (s *MCPServerE2ETestSuite) TearDownTest() {
	s.client.Close()
	s.lis.Close() // This will stop the server goroutine
}

// ========== Test Cases ==========

func (s *MCPServerE2ETestSuite) TestGetVersion() {
	var result string
	err := s.client.Call(context.Background(), "get_version", nil, &result)
	s.Require().NoError(err)
	s.Contains(result, "test")
}

func (s *MCPServerE2ETestSuite) TestRuleCRUD_FullCycle() {
	// ADD with ID
	addParams := map[string]interface{}{"name": "Test Rule", "content": "Test Content", "id": "rule-1"}
	var addResult interface{}
	err := s.client.Call(context.Background(), "add_rule", addParams, &addResult)
	s.Require().NoError(err)

	// GET
	var getResult interface{}
	err = s.client.Call(context.Background(), "get_rule", map[string]string{"name": "Test Rule"}, &getResult)
	s.Require().NoError(err)
	s.Contains(fmt.Sprintf("%v", getResult), "rule-1")
}

func (s *MCPServerE2ETestSuite) TestOutputCRUD_FullCycle() {
	// ADD with Type
	addParams := map[string]interface{}{"path": "test.md", "type": "agent"}
	var addResult interface{}
	err := s.client.Call(context.Background(), "add_output", addParams, &addResult)
	s.Require().NoError(err)

	// GET
	var getResult interface{}
	err = s.client.Call(context.Background(), "get_output", map[string]string{"path": "test.md"}, &getResult)
	s.Require().NoError(err)
	s.Contains(fmt.Sprintf("%v", getResult), "type:agent")
}

func (s *MCPServerE2ETestSuite) TestMCPServerCRUD_FullCycle() {
	// ADD with ID
	addParams := map[string]interface{}{"name": "test-server", "command": "test", "id": "server-1"}
	var addResult interface{}
	err := s.client.Call(context.Background(), "add_mcp_server", addParams, &addResult)
	s.Require().NoError(err)

	// GET
	var getResult interface{}
	err = s.client.Call(context.Background(), "get_mcp_server", map[string]string{"name": "test-server"}, &getResult)
	s.Require().NoError(err)
	s.Contains(fmt.Sprintf("%v", getResult), "server-1")
}

func (s *MCPServerE2ETestSuite) TestCommandCRUD_FullCycle() {
	// ADD with ID
	addParams := map[string]interface{}{"name": "test-cmd", "description": "Test Desc", "id": "cmd-1"}
	var addResult interface{}
	err := s.client.Call(context.Background(), "add_command", addParams, &addResult)
	s.Require().NoError(err)

	// GET
	var getResult interface{}
	err = s.client.Call(context.Background(), "get_command", map[string]string{"name": "test-cmd"}, &getResult)
	s.Require().NoError(err)
	s.Contains(fmt.Sprintf("%v", getResult), "cmd-1")
}

func (s *MCPServerE2ETestSuite) TestInitProject() {
	// Create a new directory for the init test
	initDir := testutil.CreateTempDir(s.T())
	// We need to change the working directory for the handler to work correctly
	wd, _ := os.Getwd()
	os.Chdir(initDir)
	defer os.Chdir(wd)

	params := map[string]interface{}{
		"project_name": "MCP-Initialized-Project",
		"providers":    []string{"claude", "continue-dev"},
		"with_agents":  true,
	}
	var result interface{}
	err := s.client.Call(context.Background(), "init_project", params, &result)
	s.Require().NoError(err)

	// Assert that the files were created
	configPath := filepath.Join(initDir, "ai_rulez.yaml")
	s.True(testutil.FileExists(s.T(), configPath))
	configPyPath := filepath.Join(initDir, "config.py")
	s.True(testutil.FileExists(s.T(), configPyPath), "config.py for continue-dev should be created")

	// Assert content of ai_rulez.yaml
	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "name: MCP-Initialized-Project")
	s.Contains(content, "path: .claude/agents/")
	s.Contains(content, "path: .continue/ai_rulez_agents.py")
}

func (s *MCPServerE2ETestSuite) TestGenerateAndValidate() {
	// VALIDATE
	var validateResult interface{}
	err := s.client.Call(context.Background(), "validate_config", nil, &validateResult)
	s.Require().NoError(err)
	s.Contains(fmt.Sprintf("%v", validateResult), "valid:true")

	// GENERATE
	var generateResult interface{}
	err = s.client.Call(context.Background(), "generate_outputs", nil, &generateResult)
	s.Require().NoError(err)
	s.Contains(fmt.Sprintf("%v", generateResult), "CLAUDE.md")
}
