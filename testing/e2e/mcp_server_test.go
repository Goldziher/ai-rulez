package e2e

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/internal/mcp"
	"github.com/Goldziher/ai-rulez/testing/e2e/testutil"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/suite"
	jsonrpc "github.com/ybbus/jsonrpc/v3"
)

type MCPServerE2ETestSuite struct {
	suite.Suite
	workingDir string
	server     *mcp.Server
	client     jsonrpc.RPCClient
	httpServer *http.Server
	serverURL  string
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

	// Create HTTP server from MCP server
	mcpHTTPServer := server.NewStreamableHTTPServer(s.server.GetMCPServer())

	// Set up HTTP server URL
	s.serverURL = fmt.Sprintf("http://localhost:%d", lis.Addr().(*net.TCPAddr).Port)

	// Start the HTTP server
	s.httpServer = &http.Server{
		Addr:    lis.Addr().String(),
		Handler: mcpHTTPServer,
	}

	go func() {
		_ = s.httpServer.Serve(lis)
	}()

	// Wait a moment for the server to start
	time.Sleep(100 * time.Millisecond)

	// Create a JSON-RPC client to connect to the HTTP server
	s.client = jsonrpc.NewClient(s.serverURL)
}

func (s *MCPServerE2ETestSuite) TearDownTest() {
	// Shutdown the HTTP server gracefully
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(ctx)
	}
}

// ========== Test Cases ==========

func (s *MCPServerE2ETestSuite) TestGetVersion() {
	var result string
	resp, err := s.client.Call(context.Background(), "get_version", nil)
	s.Require().NoError(err)
	s.Require().NoError(resp.GetObject(&result))
	s.Contains(result, "test")
}

func (s *MCPServerE2ETestSuite) TestRuleCRUD_FullCycle() {
	// ADD with ID
	addParams := map[string]interface{}{"name": "Test Rule", "content": "Test Content", "id": "rule-1"}
	var addResult interface{}
	resp, err := s.client.Call(context.Background(), "add_rule", addParams)
	s.Require().NoError(err)
	s.Require().NoError(resp.GetObject(&addResult))

	// GET
	var getResult interface{}
	resp, err = s.client.Call(context.Background(), "get_rule", map[string]string{"name": "Test Rule"})
	s.Require().NoError(err)
	s.Require().NoError(resp.GetObject(&getResult))
	s.Contains(fmt.Sprintf("%v", getResult), "rule-1")
}

func (s *MCPServerE2ETestSuite) TestOutputCRUD_FullCycle() {
	// ADD with Type
	addParams := map[string]interface{}{"path": "test.md", "type": "agent"}
	var addResult interface{}
	resp, err := s.client.Call(context.Background(), "add_output", addParams)
	s.Require().NoError(err)
	s.Require().NoError(resp.GetObject(&addResult))

	// GET
	var getResult interface{}
	resp, err = s.client.Call(context.Background(), "get_output", map[string]string{"path": "test.md"})
	s.Require().NoError(err)
	s.Require().NoError(resp.GetObject(&getResult))
	s.Contains(fmt.Sprintf("%v", getResult), "type:agent")
}

func (s *MCPServerE2ETestSuite) TestMCPServerCRUD_FullCycle() {
	// ADD with ID
	addParams := map[string]interface{}{"name": "test-server", "command": "test", "id": "server-1"}
	var addResult interface{}
	resp, err := s.client.Call(context.Background(), "add_mcp_server", addParams)
	s.Require().NoError(err)
	s.Require().NoError(resp.GetObject(&addResult))

	// GET
	var getResult interface{}
	resp, err = s.client.Call(context.Background(), "get_mcp_server", map[string]string{"name": "test-server"})
	s.Require().NoError(err)
	s.Require().NoError(resp.GetObject(&getResult))
	s.Contains(fmt.Sprintf("%v", getResult), "server-1")
}

func (s *MCPServerE2ETestSuite) TestCommandCRUD_FullCycle() {
	// ADD with ID
	addParams := map[string]interface{}{"name": "test-cmd", "description": "Test Desc", "id": "cmd-1"}
	var addResult interface{}
	resp, err := s.client.Call(context.Background(), "add_command", addParams)
	s.Require().NoError(err)
	s.Require().NoError(resp.GetObject(&addResult))

	// GET
	var getResult interface{}
	resp, err = s.client.Call(context.Background(), "get_command", map[string]string{"name": "test-cmd"})
	s.Require().NoError(err)
	s.Require().NoError(resp.GetObject(&getResult))
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
	resp, err := s.client.Call(context.Background(), "init_project", params)
	s.Require().NoError(err)
	s.Require().NoError(resp.GetObject(&result))

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
	resp, err := s.client.Call(context.Background(), "validate_config", nil)
	s.Require().NoError(err)
	s.Require().NoError(resp.GetObject(&validateResult))
	s.Contains(fmt.Sprintf("%v", validateResult), "valid:true")

	// GENERATE
	var generateResult interface{}
	resp, err = s.client.Call(context.Background(), "generate_outputs", nil)
	s.Require().NoError(err)
	s.Require().NoError(resp.GetObject(&generateResult))
	s.Contains(fmt.Sprintf("%v", generateResult), "CLAUDE.md")
}
