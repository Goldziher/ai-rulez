package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/tests/e2e/testutil"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type MCPServerTestSuite struct {
	suite.Suite
	workingDir string
}

func TestMCPServerSuite(t *testing.T) {
	suite.Run(t, new(MCPServerTestSuite))
}

func (s *MCPServerTestSuite) SetupTest() {
	s.workingDir = testutil.CreateTempDir(s.T())
	testutil.SetupV3BasicConfig(s.T(), s.workingDir)
}

func (s *MCPServerTestSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

func (s *MCPServerTestSuite) TestServerStartupAndShutdown() {
	s.T().Skip("Skipping until MCP test infrastructure updated for v3.5.0")
	client := testutil.StartMCPServer(s.T(), s.workingDir)
	defer client.Close()

	response := client.GetInfo(s.T())
	response.AssertToolSuccess(s.T())

	s.NotNil(response.Result)
	if response.Result != nil && len(response.Result.Content) > 0 {
		s.Contains(response.Result.Content[0].Text, "capabilities")
	}
}

func (s *MCPServerTestSuite) TestServerInitialization() {
	s.T().Skip("Skipping until MCP test infrastructure updated for v3.5.0")
	client := testutil.StartMCPServer(s.T(), s.workingDir)
	defer client.Close()

	response := client.GetInfo(s.T())
	response.AssertToolSuccess(s.T())

	s.NotNil(response.Result, "Should have result")
}

func (s *MCPServerTestSuite) TestListTools() {
	s.T().Skip("Skipping until MCP test infrastructure updated for v3.5.0")
	client := testutil.StartMCPServer(s.T(), s.workingDir)
	defer client.Close()

	response := client.ListTools(s.T())
	response.AssertToolSuccess(s.T())

	s.NotNil(response.Result, "Should have result")
}

func (s *MCPServerTestSuite) TestServerWithInvalidConfig() {
	s.T().Skip("Skipping until MCP test infrastructure updated for v3.5.0")
	testutil.SetupV3InvalidConfig(s.T(), s.workingDir)

	client := testutil.StartMCPServer(s.T(), s.workingDir)
	defer client.Close()

	response := client.CallTool(s.T(), "validate_config", map[string]interface{}{})
	// Invalid YAML config which should cause tool to fail
	// Application errors are returned as successful responses with error content
	response.AssertToolSuccess(s.T())
	s.NotNil(response.Result)
}

func (s *MCPServerTestSuite) TestServerWithoutConfig() {
	s.T().Skip("Skipping until MCP test infrastructure updated for v3.5.0")
	emptyDir := testutil.CreateTempDir(s.T())

	client := testutil.StartMCPServer(s.T(), emptyDir)
	defer client.Close()

	response := client.CallTool(s.T(), "validate_config", map[string]interface{}{})
	// Missing V3 config returns error as response content
	response.AssertToolSuccess(s.T())
	s.NotNil(response.Result)
}

func (s *MCPServerTestSuite) TestConcurrentRequests() {
	client := testutil.StartMCPServer(s.T(), s.workingDir)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results := make(chan *testutil.MCPResponse, 5)
	errors := make(chan error, 5)

	for i := 0; i < 5; i++ {
		go func() {
			response := client.CallTool(s.T(), "get_version", map[string]interface{}{})
			if response.Error != nil {
				errors <- fmt.Errorf("tool error: %s", response.Error.Message)
			} else {
				results <- response
			}
		}()
	}

	successCount := 0
	errorCount := 0

	for i := 0; i < 5; i++ {
		select {
		case <-results:
			successCount++
		case <-errors:
			errorCount++
		case <-ctx.Done():
			s.Fail("Concurrent requests timed out")
		}
	}

	s.Equal(5, successCount, "All concurrent requests should succeed")
	s.Equal(0, errorCount, "No requests should error")
}

func (s *MCPServerTestSuite) TestServerErrorHandling() {
	s.T().Skip("Skipping until MCP test infrastructure updated for v3.5.0")
	client := testutil.StartMCPServer(s.T(), s.workingDir)
	defer client.Close()

	response := client.CallTool(s.T(), "nonexistent_tool", map[string]interface{}{})
	// Tool not found returns proper MCP error with "unknown tool" message
	response.AssertToolError(s.T(), "unknown tool")
}

func (s *MCPServerTestSuite) TestServerMemoryUsage() {
	s.T().Skip("Skipping until MCP test infrastructure updated for v3.5.0")
	client := testutil.StartMCPServer(s.T(), s.workingDir)
	defer client.Close()

	for i := 0; i < 100; i++ {
		response := client.CallTool(s.T(), "get_version", map[string]interface{}{})
		response.AssertToolSuccess(s.T())

		if i%10 == 0 {
			time.Sleep(1 * time.Millisecond)
		}
	}

	response := client.CallTool(s.T(), "get_version", map[string]interface{}{})
	response.AssertToolSuccess(s.T())
}

func (s *MCPServerTestSuite) TestServerCustomConfigPath() {
	s.T().Skip("Skipping until MCP test infrastructure updated for v3.5.0")
	// Create an additional .ai-rulez in a custom subdirectory
	customDir := filepath.Join(s.workingDir, "custom")
	err := os.MkdirAll(customDir, 0o755)
	require.NoError(s.T(), err)

	// Set up custom V3 config
	testutil.SetupV3BasicConfig(s.T(), customDir)

	// Test the original config still works
	client := testutil.StartMCPServer(s.T(), s.workingDir)
	defer client.Close()

	response := client.CallTool(s.T(), "validate_config", map[string]interface{}{})
	response.AssertToolSuccess(s.T())
}
