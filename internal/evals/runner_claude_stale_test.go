package evals

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func staleAggregate(t *testing.T, dir string) {
	t.Helper()
	doc, err := json.Marshal(map[string]any{"cases": []map[string]any{
		{"name": "fires", "arms": map[string]any{"with": []any{claudeRunJSON(true, true, 0.2, "")}}},
	}})
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "evals", "old-case"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "aggregate.json"), doc, 0o600))
}

func TestClaudePluginEval_StaleResultsInAKeptDirAreNeverScored(t *testing.T) {
	// Arrange: a previous run left aggregate.json and a case directory behind.
	dir := t.TempDir()
	staleAggregate(t, dir)
	runner := &ClaudePluginEval{KeepDir: dir, Exec: func(context.Context, string, []string, io.Writer, io.Writer) error {
		assert.NoFileExists(t, filepath.Join(dir, "aggregate.json"))
		assert.NoDirExists(t, filepath.Join(dir, "evals", "old-case"))
		return os.ErrPermission // the tool fails and writes nothing
	}}

	// Act
	_, err := runner.Run(context.Background(), sampleRequest(t))

	// Assert
	assert.ErrorContains(t, err, "claude plugin eval failed")
}

func TestClaudePluginEval_TimeoutIsAnErrorEvenWhenAResultFileExists(t *testing.T) {
	dir := t.TempDir()
	runner := &ClaudePluginEval{KeepDir: dir, Timeout: 20 * time.Millisecond,
		Exec: func(ctx context.Context, _ string, args []string, _, _ io.Writer) error {
			staleAggregate(t, dir) // a partial result written before the kill
			<-ctx.Done()
			return nil // the tool exits cleanly after being killed
		}}

	_, err := runner.Run(context.Background(), sampleRequest(t))

	require.ErrorContains(t, err, "timed out after")
	assert.NotContains(t, err.Error(), "%!w")
}
