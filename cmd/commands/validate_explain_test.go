package commands

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunExplainText(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, runExplain(&buf, "AR001", ""))
	out := buf.String()
	for _, want := range []string{"AR001 secret-detected", "Default severity: error", "Why it matters", "Bad", "Good", "ai-rulez-lint-ignore: AR001", "#ar001-secret-detected"} {
		assert.Contains(t, out, want)
	}
}

func TestRunExplainJSONByName(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, runExplain(&buf, "link-unresolved", formatJSON))
	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	assert.Equal(t, "AR201", got["code"])
	assert.NotEmpty(t, got["why"])
}

func TestRunExplainUnknownRule(t *testing.T) {
	assert.Error(t, runExplain(&bytes.Buffer{}, "AR999", ""))
}

func TestValidateHasExplainFlag(t *testing.T) {
	assert.NotNil(t, ValidateCmd.Flags().Lookup("explain"))
}
