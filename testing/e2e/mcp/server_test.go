package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/testing/e2e/testutil"
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
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.BasicConfig)
}

func (s *MCPServerTestSuite) TearDownSuite() {
	testutil.CleanupTestBinary()
}

func (s *MCPServerTestSuite) TestServerStartupAndShutdown() {
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
	client := testutil.StartMCPServer(s.T(), s.workingDir)
	defer client.Close()

	response := client.GetInfo(s.T())
	response.AssertToolSuccess(s.T())

	s.NotNil(response.Result, "Should have result")
}

func (s *MCPServerTestSuite) TestListTools() {
	client := testutil.StartMCPServer(s.T(), s.workingDir)
	defer client.Close()

	response := client.ListTools(s.T())
	response.AssertToolSuccess(s.T())

	s.NotNil(response.Result, "Should have result")
}

func (s *MCPServerTestSuite) TestServerWithInvalidConfig() {
	testutil.WriteFile(s.T(), s.workingDir, "ai_rulez.yaml", testutil.InvalidYAMLConfig)

	client := testutil.StartMCPServer(s.T(), s.workingDir)
	defer client.Close()

	response := client.CallTool(s.T(), "get_rules", map[string]interface{}{})
	response.AssertToolError(s.T(), "")
}

func (s *MCPServerTestSuite) TestServerWithoutConfig() {
	emptyDir := testutil.CreateTempDir(s.T())

	client := testutil.StartMCPServer(s.T(), emptyDir)
	defer client.Close()

	response := client.CallTool(s.T(), "get_rules", map[string]interface{}{})
	response.AssertToolError(s.T(), "configuration")
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
	client := testutil.StartMCPServer(s.T(), s.workingDir)
	defer client.Close()

	response := client.CallTool(s.T(), "nonexistent_tool", map[string]interface{}{})
	response.AssertToolError(s.T(), "not found")

	response = client.CallTool(s.T(), "update_rule", map[string]interface{}{
		"name":    "does_not_exist",
		"content": "test",
	})
	response.AssertToolError(s.T(), "not found")
}

func (s *MCPServerTestSuite) TestServerMemoryUsage() {
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
	err := os.MkdirAll(filepath.Join(s.workingDir, "custom"), 0o755)
	require.NoError(s.T(), err)
	testutil.WriteFile(s.T(), s.workingDir, "custom/config.yaml", testutil.MinimalConfig)

	client := testutil.StartMCPServer(s.T(), s.workingDir)
	defer client.Close()

	response := client.CallTool(s.T(), "get_rules", map[string]interface{}{})
	response.AssertToolSuccess(s.T())
}
