package testutil

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type MCPClient struct {
	cmd    *exec.Cmd
	stdin  *bufio.Writer
	stdout *bufio.Scanner
	ctx    context.Context
	cancel context.CancelFunc
	mutex  sync.Mutex
}

type MCPResponse struct {
	ID     interface{} `json:"id"`
	Result *MCPResult  `json:"result,omitempty"`
	Error  *MCPError   `json:"error,omitempty"`
}

type MCPResult struct {
	Content []MCPContent `json:"content,omitempty"`
}

type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func StartMCPServer(t *testing.T, workingDir string) *MCPClient {
	t.Helper()

	binaryPath := SetupTestBinary(t)

	ctx, cancel := context.WithCancel(context.Background())

	//nolint:gosec // G204: Test utility needs to run MCP server with variable path
	cmd := exec.CommandContext(ctx, binaryPath, "mcp")
	cmd.Dir = workingDir

	stdin, err := cmd.StdinPipe()
	require.NoError(t, err, "Failed to create stdin pipe")

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err, "Failed to create stdout pipe")

	err = cmd.Start()
	require.NoError(t, err, "Failed to start MCP server")

	client := &MCPClient{
		cmd:    cmd,
		stdin:  bufio.NewWriter(stdin),
		stdout: bufio.NewScanner(stdout),
		ctx:    ctx,
		cancel: cancel,
	}

	client.initialize(t)

	return client
}

func (c *MCPClient) Close() {
	if c.cancel != nil {
		c.cancel()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		//nolint:errcheck,gosec
		c.cmd.Process.Kill()
		//nolint:errcheck,gosec
		c.cmd.Wait()
	}
}

func (c *MCPClient) CallTool(t *testing.T, toolName string, params map[string]interface{}) *MCPResponse {
	t.Helper()

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      time.Now().UnixNano(),
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      toolName,
			"arguments": params,
		},
	}

	return c.sendRequest(t, request)
}

func (c *MCPClient) ListTools(t *testing.T) *MCPResponse {
	t.Helper()

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      time.Now().UnixNano(),
		"method":  "tools/list",
	}

	return c.sendRequest(t, request)
}

func (c *MCPClient) GetInfo(t *testing.T) *MCPResponse {
	t.Helper()

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      time.Now().UnixNano(),
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "ai-rulez-test-client",
				"version": "1.0.0",
			},
		},
	}

	return c.sendRequest(t, request)
}

func (c *MCPClient) initialize(t *testing.T) {
	t.Helper()

	response := c.GetInfo(t)
	require.Nil(t, response.Error, "MCP initialization should succeed")

	notification := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}

	data, err := json.Marshal(notification)
	require.NoError(t, err)

	_, err = c.stdin.WriteString(string(data) + "\n")
	require.NoError(t, err)

	err = c.stdin.Flush()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
}

func (c *MCPClient) sendRequest(t *testing.T, request map[string]interface{}) *MCPResponse {
	t.Helper()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	data, err := json.Marshal(request)
	require.NoError(t, err, "Failed to marshal request")

	_, err = c.stdin.WriteString(string(data) + "\n")
	require.NoError(t, err, "Failed to write request")

	err = c.stdin.Flush()
	require.NoError(t, err, "Failed to flush request")

	responseChan := make(chan *MCPResponse, 1)
	errorChan := make(chan error, 1)

	go func() {
		if c.stdout.Scan() {
			line := c.stdout.Text()
			var response MCPResponse
			if err := json.Unmarshal([]byte(line), &response); err != nil {
				errorChan <- fmt.Errorf("failed to unmarshal response: %w", err)
				return
			}
			responseChan <- &response
		} else {
			errorChan <- fmt.Errorf("failed to read response")
		}
	}()

	select {
	case response := <-responseChan:
		return response
	case err := <-errorChan:
		require.NoError(t, err, "Failed to receive MCP response")
		return nil
	case <-time.After(5 * time.Second):
		t.Fatal("MCP request timed out")
		return nil
	}
}

func (r *MCPResponse) GetParsedResult(t *testing.T) map[string]interface{} {
	t.Helper()

	parsed, err := r.GetParsedContent()
	require.NoError(t, err, "Failed to parse response content")

	return parsed
}

func (r *MCPResponse) GetParsedContent() (map[string]interface{}, error) {
	if r.Result == nil || len(r.Result.Content) == 0 {
		return nil, fmt.Errorf("no content in response")
	}

	textContent := r.Result.Content[0].Text

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(textContent), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse JSON content: %w", err)
	}

	return parsed, nil
}

func (r *MCPResponse) GetParsedArray() ([]interface{}, error) {
	if r.Result == nil || len(r.Result.Content) == 0 {
		return nil, fmt.Errorf("no content in response")
	}

	textContent := r.Result.Content[0].Text

	var parsed []interface{}
	if err := json.Unmarshal([]byte(textContent), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse JSON array content: %w", err)
	}

	return parsed, nil
}

func (r *MCPResponse) GetParsedArrayResult(t *testing.T) []interface{} {
	t.Helper()

	parsed, err := r.GetParsedArray()
	require.NoError(t, err, "Failed to parse response array content")

	return parsed
}

func (r *MCPResponse) AssertToolSuccess(t *testing.T) {
	t.Helper()

	require.Nil(t, r.Error, "Tool call should succeed, got error: %+v", r.Error)
	require.NotNil(t, r.Result, "Tool call should return result")
}

func (r *MCPResponse) AssertToolError(t *testing.T, expectedMessage string) {
	t.Helper()

	require.NotNil(t, r.Error, "Tool call should fail")
	if expectedMessage != "" {
		require.Contains(t, strings.ToLower(r.Error.Message), strings.ToLower(expectedMessage),
			"Error message should contain expected text")
	}
}

func (r *MCPResponse) GetResultString(t *testing.T, key string) string {
	t.Helper()

	r.AssertToolSuccess(t)

	parsed, err := r.GetParsedContent()
	require.NoError(t, err, "Failed to parse response content")

	value, exists := parsed[key]
	require.True(t, exists, "Result should contain key: %s", key)

	str, ok := value.(string)
	require.True(t, ok, "Value should be string: %+v", value)

	return str
}
