package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/tests/e2e/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const reinitializeTimeout = 30 * time.Second

// initializeLine is the raw JSON-RPC frame from issue #158. It is written
// verbatim (rather than through the SDK client) because the SDK client will
// never send `initialize` twice, which is exactly the case under test.
func initializeLine(id int) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"initialize","params":`+
		`{"protocolVersion":"2025-06-18","capabilities":{},`+
		`"clientInfo":{"name":"reinit-test","version":"0.1"}}}`, id)
}

type rawRPCResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// TestServerToleratesRepeatedInitialize reproduces issue #158: MCP hosts that
// retry or reconnect (Claude Code's client, an mcpm/fastmcp bridge shared
// between consumers) re-send `initialize` on an established stdio session. The
// server must answer the repeat instead of wedging the session.
func TestServerToleratesRepeatedInitialize(t *testing.T) {
	t.Parallel()

	binaryPath := testutil.SetupTestBinary(t)
	workingDir := testutil.CreateTempDir(t)
	testutil.SetupBasicConfig(t, workingDir)

	ctx, cancel := context.WithTimeout(context.Background(), reinitializeTimeout)
	defer cancel()

	//nolint:gosec // G204: e2e test runs the built CLI from a computed path.
	cmd := exec.CommandContext(ctx, binaryPath, "mcp")
	cmd.Dir = workingDir
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)

	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	_, err = fmt.Fprintf(stdin, "%s\n%s\n", initializeLine(1), initializeLine(2))
	require.NoError(t, err, "failed to write initialize frames; stderr=%q", stderr.String())

	reader := bufio.NewReader(stdout)
	for _, wantID := range []int{1, 2} {
		line, readErr := reader.ReadBytes('\n')
		require.NoError(t, readErr, "no response for initialize id=%d; stderr=%q", wantID, stderr.String())

		var response rawRPCResponse
		require.NoError(t, json.Unmarshal(line, &response), "malformed response: %s", line)

		assert.Equal(t, wantID, response.ID)
		assert.Nil(t, response.Error, "initialize id=%d must not error; stderr=%q", wantID, stderr.String())
		assert.NotEmpty(t, response.Result, "initialize id=%d must return a result", wantID)
	}

	require.NoError(t, stdin.Close())
}
