package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// MCPClient wraps the official Go MCP SDK client for process-based e2e tests.
// This keeps the tests exercising the built CLI over stdio without duplicating
// the SDK's JSON-RPC transport implementation in test code.
type MCPClient struct {
	session *sdkmcp.ClientSession
	cancel  context.CancelFunc
	stderr  *bytes.Buffer
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

const (
	mcpRequestTimeout  = 90 * time.Second
	mcpTextContentType = "text"
)

func StartMCPServer(t *testing.T, workingDir string) *MCPClient {
	t.Helper()

	binaryPath := SetupTestBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), mcpRequestTimeout)

	//nolint:gosec // G204: Test utility needs to run MCP server with variable path.
	cmd := exec.CommandContext(ctx, binaryPath, "mcp")
	cmd.Dir = workingDir
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

	client := sdkmcp.NewClient(&sdkmcp.Implementation{
		Name:    "ai-rulez-test-client",
		Version: "1.0.0",
	}, nil)
	session, err := client.Connect(ctx, &sdkmcp.CommandTransport{
		Command:           cmd,
		TerminateDuration: 2 * time.Second,
	}, nil)
	require.NoError(t, err, "Failed to connect to MCP server; stderr=%q", stderr.String())

	return &MCPClient{
		session: session,
		cancel:  cancel,
		stderr:  stderr,
	}
}

func (c *MCPClient) Close() {
	if c.session != nil {
		if err := c.session.Close(); err != nil && c.stderr != nil {
			fmt.Fprintf(c.stderr, "\nclose: %v", err)
		}
	}
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *MCPClient) CallTool(t *testing.T, toolName string, params map[string]interface{}) *MCPResponse {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), mcpRequestTimeout)
	defer cancel()

	result, err := c.session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      toolName,
		Arguments: params,
	})
	if err != nil {
		return &MCPResponse{
			Error: &MCPError{
				Message: fmt.Sprintf("%v; stderr=%q", err, c.stderr.String()),
			},
		}
	}

	response := convertToolResult(result)
	if result.IsError && response.Error == nil {
		response.Error = &MCPError{Message: responseText(response)}
	}
	return response
}

func (c *MCPClient) ListTools(t *testing.T) *MCPResponse {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), mcpRequestTimeout)
	defer cancel()

	result, err := c.session.ListTools(ctx, nil)
	if err != nil {
		return &MCPResponse{Error: &MCPError{Message: fmt.Sprintf("%v; stderr=%q", err, c.stderr.String())}}
	}

	content, marshalErr := json.Marshal(result)
	require.NoError(t, marshalErr, "Failed to marshal list tools result")
	return &MCPResponse{
		Result: &MCPResult{
			Content: []MCPContent{{Type: mcpTextContentType, Text: string(content)}},
		},
	}
}

func (c *MCPClient) GetInfo(t *testing.T) *MCPResponse {
	t.Helper()

	result := c.session.InitializeResult()
	content, err := json.Marshal(result)
	require.NoError(t, err, "Failed to marshal initialize result")

	return &MCPResponse{
		Result: &MCPResult{
			Content: []MCPContent{{Type: mcpTextContentType, Text: string(content)}},
		},
	}
}

func convertToolResult(result *sdkmcp.CallToolResult) *MCPResponse {
	response := &MCPResponse{Result: &MCPResult{}}
	if result == nil {
		return response
	}

	response.Result.Content = make([]MCPContent, 0, len(result.Content))
	for _, content := range result.Content {
		switch typed := content.(type) {
		case *sdkmcp.TextContent:
			response.Result.Content = append(response.Result.Content, MCPContent{
				Type: mcpTextContentType,
				Text: typed.Text,
			})
		default:
			data, err := content.MarshalJSON()
			if err != nil {
				response.Result.Content = append(response.Result.Content, MCPContent{
					Type: mcpTextContentType,
					Text: fmt.Sprintf("%v", content),
				})
				continue
			}
			response.Result.Content = append(response.Result.Content, MCPContent{
				Type: mcpTextContentType,
				Text: string(data),
			})
		}
	}
	return response
}

func responseText(response *MCPResponse) string {
	if response == nil || response.Result == nil {
		return ""
	}
	parts := make([]string, 0, len(response.Result.Content))
	for _, content := range response.Result.Content {
		parts = append(parts, content.Text)
	}
	return strings.Join(parts, "\n")
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
