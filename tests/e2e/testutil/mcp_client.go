package testutil

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// MCPClient is a minimal JSON-RPC 2.0 client for the MCP stdio transport,
// used by the e2e tests. It is correctness-focused, not performance-focused:
//
//   - One persistent reader goroutine demuxes stdout into per-request
//     channels keyed by JSON-RPC id, so notifications interleaved with
//     responses (logging, server-initiated requests) cannot be misparsed
//     as the response to an outstanding request.
//   - Notifications (frames without an id) are silently discarded.
//   - The reader's bufio.Scanner buffer is enlarged because the MCP
//     `tools/list` response is a single line that can exceed 64 KiB.
type MCPClient struct {
	cmd    *exec.Cmd
	stdin  *bufio.Writer
	ctx    context.Context
	cancel context.CancelFunc

	writeMu sync.Mutex // serializes writes to stdin

	pendingMu sync.Mutex
	pending   map[string]chan *MCPResponse // id (as JSON string) -> response channel

	readerDone chan struct{}
	readerErr  error
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
	// mcpRequestTimeout is the per-RPC deadline. Generous because cold
	// runs after a binary rebuild can take several seconds before the
	// MCP server is ready to respond.
	mcpRequestTimeout = 30 * time.Second

	// mcpReaderBufferBytes is the maximum length of a single stdout line
	// the reader will accept. The MCP `tools/list` response is one big
	// JSON object on a single line; with the current 35+ tools it is
	// already ~18 KiB, so we leave plenty of headroom.
	mcpReaderBufferBytes = 1 << 20 // 1 MiB
)

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
		cmd:        cmd,
		stdin:      bufio.NewWriter(stdin),
		ctx:        ctx,
		cancel:     cancel,
		pending:    map[string]chan *MCPResponse{},
		readerDone: make(chan struct{}),
	}

	go client.readLoop(stdout)

	client.initialize(t)

	return client
}

// readLoop is the single owner of stdout. It runs until stdout closes
// (server exit) and routes responses to whichever sendRequest is
// waiting on that id. Notifications (no id) are dropped.
func (c *MCPClient) readLoop(stdout io.Reader) {
	defer close(c.readerDone)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), mcpReaderBufferBytes)

	for scanner.Scan() {
		line := scanner.Bytes()
		var msg MCPResponse
		if err := json.Unmarshal(line, &msg); err != nil {
			// Malformed frame — record but keep reading; a subsequent
			// well-formed response should still arrive for any caller.
			c.readerErr = fmt.Errorf("decode mcp frame: %w (raw=%q)", err, string(line))
			continue
		}
		if msg.ID == nil {
			// Notification (e.g. notifications/message). Discard.
			continue
		}
		key := jsonRPCIDKey(msg.ID)

		c.pendingMu.Lock()
		ch, ok := c.pending[key]
		if ok {
			delete(c.pending, key)
		}
		c.pendingMu.Unlock()

		if !ok {
			// Response for an unknown id (likely a request that already
			// timed out). Drop it — no one is listening.
			continue
		}
		ch <- &msg
	}

	if err := scanner.Err(); err != nil {
		c.readerErr = err
	}

	// Stdout closed: fail any still-waiting requests so callers don't
	// block forever on a dead server.
	c.pendingMu.Lock()
	for key, ch := range c.pending {
		close(ch)
		delete(c.pending, key)
	}
	c.pendingMu.Unlock()
}

// jsonRPCIDKey normalizes a decoded JSON-RPC id back to its on-the-wire
// form so requests and responses key on the same string.
//
// JSON numbers come out of encoding/json as float64; rendering them with
// %v would produce e.g. "1e+09" for large nanoseconds. Re-marshaling
// guarantees identical formatting on both sides.
func jsonRPCIDKey(id interface{}) string {
	b, err := json.Marshal(id)
	if err != nil {
		return fmt.Sprintf("%v", id)
	}
	return string(b)
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
	// Wait for reader to drain so its goroutine doesn't leak past test end.
	select {
	case <-c.readerDone:
	case <-time.After(2 * time.Second):
	}
}

func (c *MCPClient) CallTool(t *testing.T, toolName string, params map[string]interface{}) *MCPResponse {
	t.Helper()
	return c.sendRequest(t, "tools/call", map[string]interface{}{
		"name":      toolName,
		"arguments": params,
	})
}

func (c *MCPClient) ListTools(t *testing.T) *MCPResponse {
	t.Helper()
	return c.sendRequest(t, "tools/list", nil)
}

func (c *MCPClient) GetInfo(t *testing.T) *MCPResponse {
	t.Helper()
	return c.sendRequest(t, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "ai-rulez-test-client",
			"version": "1.0.0",
		},
	})
}

func (c *MCPClient) initialize(t *testing.T) {
	t.Helper()

	response := c.GetInfo(t)
	require.Nil(t, response.Error, "MCP initialization should succeed")

	c.sendNotification(t, "notifications/initialized", nil)
}

// sendNotification fires-and-forgets a JSON-RPC notification (no id, no
// response expected).
func (c *MCPClient) sendNotification(t *testing.T, method string, params interface{}) {
	t.Helper()
	frame := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		frame["params"] = params
	}
	data, err := json.Marshal(frame)
	require.NoError(t, err)

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.stdin.WriteString(string(data) + "\n")
	require.NoError(t, err)
	require.NoError(t, c.stdin.Flush())
}

// sendRequest registers a response channel for a fresh id, writes the
// request, and waits for the matching response (or timeout / server exit).
func (c *MCPClient) sendRequest(t *testing.T, method string, params interface{}) *MCPResponse {
	t.Helper()

	id := time.Now().UnixNano()
	key := jsonRPCIDKey(id)
	ch := make(chan *MCPResponse, 1)

	c.pendingMu.Lock()
	c.pending[key] = ch
	c.pendingMu.Unlock()

	frame := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		frame["params"] = params
	}
	data, err := json.Marshal(frame)
	require.NoError(t, err, "Failed to marshal request")

	c.writeMu.Lock()
	_, err = c.stdin.WriteString(string(data) + "\n")
	if err == nil {
		err = c.stdin.Flush()
	}
	c.writeMu.Unlock()
	require.NoError(t, err, "Failed to write request")

	select {
	case response, ok := <-ch:
		if !ok {
			// Reader closed the channel without a response (server exited).
			c.cleanupPending(key)
			t.Fatalf("MCP server exited before responding to %s (reader err: %v)", method, c.readerErr)
			return nil
		}
		return response
	case <-time.After(mcpRequestTimeout):
		c.cleanupPending(key)
		t.Fatalf("MCP request timed out after %s: method=%s", mcpRequestTimeout, method)
		return nil
	}
}

func (c *MCPClient) cleanupPending(key string) {
	c.pendingMu.Lock()
	delete(c.pending, key)
	c.pendingMu.Unlock()
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

func (r *MCPResponse) GetTextContent(t *testing.T) string {
	t.Helper()

	require.NotNil(t, r.Result, "Response should have a result")
	require.NotEmpty(t, r.Result.Content, "Result should have content")

	return strings.TrimSpace(r.Result.Content[0].Text)
}
