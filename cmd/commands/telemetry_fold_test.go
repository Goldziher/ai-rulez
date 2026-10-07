package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoot_UsageAndReportAreFoldedIntoTelemetry(t *testing.T) {
	for _, removed := range []string{"usage", "report"} {
		for _, c := range RootCmd.Commands() {
			assert.NotEqual(t, removed, c.Name(), "%q was folded into telemetry without an alias", removed)
		}
	}
	for _, path := range [][]string{{"feedback"}, {"report"}, {"report", "evals"}, {"hook"}, {"record"}} {
		found, _, err := TelemetryCmd.Find(path)
		require.NoError(t, err)
		assert.Equal(t, path[len(path)-1], found.Name(), "telemetry %v must exist", path)
	}
}

func TestTelemetryFeedback_RecordsHarnessAndRole(t *testing.T) {
	resetEnrichFlags(t)
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("CLAUDE_PROJECT_DIR", root)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".ai-rulez"), 0o750))
	cmd, _, err := TelemetryCmd.Find([]string{"feedback"})
	require.NoError(t, err)
	require.Equal(t, "feedback", cmd.Name())
	require.NoError(t, cmd.Flags().Set("harness", "codex"))
	require.NoError(t, cmd.Flags().Set("role", "myrole"))
	feedbackKind = "great"
	var out bytes.Buffer
	cmd.SetOut(&out)
	require.NoError(t, cmd.RunE(cmd, []string{"alpha"}))
	entries, _, err := usage.ReadFeedback(filepath.Join(root, ".ai-rulez", "local", usage.FeedbackFileName))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "codex", entries[0].Harness)
	assert.Equal(t, "myrole", entries[0].Role)
}
