package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/render"
)

func TestVerifyWithoutAModePointsAtGenerateCheck(t *testing.T) {
	// Act
	err := errVerifyNeedsMode

	// Assert
	assert.Equal(t, exitFailure, exitCodeFor(err))
	assert.Equal(t, "run `ai-rulez generate --check` to find generated files that differ from their sources; verify checks signatures and provenance: --attestation, --approvals, --self or --plugin", errorHintOf(err))
}

func TestFinishPluginVerifyJSON(t *testing.T) {
	tests := []struct {
		name       string
		doc        pluginVerifyDoc
		wantCode   int
		wantStatus string
		wantStderr bool
	}{
		{"ok", pluginVerifyDoc{Status: pluginVerifyOK, Configs: 2}, exitOK, "ok", false},
		{"skipped", pluginVerifyDoc{Status: pluginVerifySkipped}, exitOK, "skipped", false},
		{"drift", pluginVerifyDoc{Status: pluginVerifyDrift, Configs: 1, err: failWithCode(pluginVerifyExitCode(generator.ErrPluginDrift), generator.ErrPluginDrift)}, exitFindings, "drift", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var stdout, stderr bytes.Buffer
			out := render.New(&stdout, &stderr, false).WithFormat(formatJSON)

			// Act
			err := finishPluginVerify(out, tt.doc)

			// Assert
			assert.Equal(t, tt.wantCode, codeOf(err))
			var got pluginVerifyDoc
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &got), stdout.String())
			assert.Equal(t, 1, got.SchemaVersion)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantStderr, stderr.Len() > 0)
		})
	}
}

func TestFinishPluginVerifyLeavesATooledFailureToTheRoot(t *testing.T) {
	// Arrange
	var stdout, stderr bytes.Buffer
	out := render.New(&stdout, &stderr, false).WithFormat(formatJSON)
	failure := fail(errors.New("render expected plugin outputs"))

	// Act
	err := finishPluginVerify(out, pluginVerifyDoc{err: failure})

	// Assert: nothing written here, the root renders the error document once
	assert.Equal(t, failure, err)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestFinishPluginVerifyTextReturnsTheOutcome(t *testing.T) {
	// Arrange
	var stdout, stderr bytes.Buffer
	out := render.New(&stdout, &stderr, false)

	// Act and assert
	assert.NoError(t, finishPluginVerify(out, pluginVerifyDoc{Status: pluginVerifyOK}))
	drift := failWithCode(exitFindings, generator.ErrPluginDrift)
	assert.Equal(t, drift, finishPluginVerify(out, pluginVerifyDoc{err: drift}))
	assert.Empty(t, stdout.String())
}
