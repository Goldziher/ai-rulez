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
	"github.com/ybbus/jsonrpc/v3"
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

	originalDir, err := os.Getwd()
	s.Require().NoError(err)

	err = os.Chdir(s.workingDir)
	s.Require().NoError(err)

	s.server = mcp.NewServer("test")

	err = os.Chdir(originalDir)
	s.Require().NoError(err)

	lis, err := net.Listen("tcp", ":0")
	s.Require().NoError(err)

	mcpHTTPServer := server.NewStreamableHTTPServer(s.server.GetMCPServer(), server.WithStateLess(true))

	workingDir := s.workingDir
	wrappedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originalDir, err := os.Getwd()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if err := os.Chdir(workingDir); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		defer func() {
			_ = os.Chdir(originalDir)
		}()

		mcpHTTPServer.ServeHTTP(w, r)
	})

	s.serverURL = fmt.Sprintf("http://localhost:%d", lis.Addr().(*net.TCPAddr).Port)

	s.httpServer = &http.Server{
		Addr:    lis.Addr().String(),
		Handler: wrappedHandler,
	}

	go func() {
		if err := s.httpServer.Serve(lis); err != nil && err != http.ErrServerClosed {
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	s.client = jsonrpc.NewClient(s.serverURL)
}

func (s *MCPServerE2ETestSuite) TearDownTest() {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(ctx)
	}
}

func (s *MCPServerE2ETestSuite) callTool(toolName string, args map[string]interface{}) string {
	params := map[string]interface{}{
		"name":      toolName,
		"arguments": args,
	}

	resp, err := s.client.Call(context.Background(), "tools/call", params)
	s.Require().NoError(err)

	var toolResult struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	s.Require().NoError(resp.GetObject(&toolResult))

	if len(toolResult.Content) > 0 {
		return toolResult.Content[0].Text
	}
	return ""
}

func (s *MCPServerE2ETestSuite) TestGetVersion() {
	params := map[string]interface{}{
		"name":      "get_version",
		"arguments": map[string]interface{}{},
	}

	resp, err := s.client.Call(context.Background(), "tools/call", params)
	s.Require().NoError(err)

	var toolResult struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	s.Require().NoError(resp.GetObject(&toolResult))

	s.Require().NotEmpty(toolResult.Content, "Tool result should have content")
	s.Contains(toolResult.Content[0].Text, "test")
}

func (s *MCPServerE2ETestSuite) TestRuleCRUD_FullCycle() {
	addParams := map[string]interface{}{"name": "Test Rule", "content": "Test Content", "id": "rule-1"}
	addResult := s.callTool("add_rule", addParams)
	s.Contains(addResult, "Added rule")

	getResult := s.callTool("get_rule", map[string]interface{}{"name": "Test Rule"})
	s.Contains(getResult, "rule-1")
}

func (s *MCPServerE2ETestSuite) TestOutputCRUD_FullCycle() {
	addParams := map[string]interface{}{"path": "test.md", "type": "agent"}
	addResult := s.callTool("add_output", addParams)
	s.Contains(addResult, "Added output")

	getResult := s.callTool("get_output", map[string]interface{}{"path": "test.md"})
	s.Contains(getResult, "agent")
}

func (s *MCPServerE2ETestSuite) TestMCPServerCRUD_FullCycle() {
	addParams := map[string]interface{}{"name": "test-server", "command": "test", "id": "server-1"}
	addResult := s.callTool("add_mcp_server", addParams)
	s.Contains(addResult, "Added mcp_servers: test-server")

	getResult := s.callTool("get_mcp_server", map[string]interface{}{"name": "test-server"})
	s.Contains(getResult, "server-1")
}

func (s *MCPServerE2ETestSuite) TestCommandCRUD_FullCycle() {
	addParams := map[string]interface{}{"name": "test-cmd", "description": "Test Desc", "id": "cmd-1"}
	addResult := s.callTool("add_command", addParams)
	s.Contains(addResult, "Added command")

	getResult := s.callTool("get_command", map[string]interface{}{"name": "test-cmd"})
	s.Contains(getResult, "cmd-1")
}

func (s *MCPServerE2ETestSuite) TestInitProject() {
	os.Remove(filepath.Join(s.workingDir, "ai_rulez.yaml"))

	params := map[string]interface{}{
		"project_name": "MCP-Initialized-Project",
		"providers":    []string{"claude", "continue-dev"},
		"with_agents":  true,
	}
	result := s.callTool("init_project", params)
	s.Contains(result, "initialized")

	configPath := filepath.Join(s.workingDir, "ai_rulez.yaml")
	s.True(testutil.FileExists(s.T(), configPath), "Config file should exist at %s", configPath)

	content := testutil.ReadFile(s.T(), configPath)
	s.Contains(content, "name: \"MCP-Initialized-Project\"")
	s.Contains(content, "- \"claude\"")
	s.Contains(content, "- \"continue-dev\"")
}

func (s *MCPServerE2ETestSuite) TestGenerateAndValidate() {
	validateResult := s.callTool("validate_config", map[string]interface{}{})
	s.Contains(validateResult, "valid")

	generateResult := s.callTool("generate_outputs", map[string]interface{}{})
	s.Contains(generateResult, "generated")
}
