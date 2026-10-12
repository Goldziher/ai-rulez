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

func TestFinishPluginVerifyReportsDriftedPaths(t *testing.T) {
	// Arrange
	drift := []generator.PluginDrift{
		{Path: "skills/a/SKILL.md", Kind: generator.PluginDriftChanged},
		{Path: "skills/b/SKILL.md", Kind: generator.PluginDriftMissing},
	}
	doc := func() pluginVerifyDoc {
		return pluginVerifyDoc{Status: pluginVerifyDrift, Configs: 1, Drift: drift,
			err: failWithCode(exitFindings, generator.ErrPluginDrift)}
	}

	t.Run("text lists each path with its kind", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		out := render.New(&stdout, &stderr, false)

		err := finishPluginVerify(out, doc())

		assert.Equal(t, exitFindings, codeOf(err))
		assert.Contains(t, stderr.String(), "changed      skills/a/SKILL.md")
		assert.Contains(t, stderr.String(), "missing      skills/b/SKILL.md")
	})

	t.Run("json carries a drift array", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		out := render.New(&stdout, &stderr, false).WithFormat(formatJSON)

		err := finishPluginVerify(out, doc())

		assert.Equal(t, exitFindings, codeOf(err))
		var got pluginVerifyDoc
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &got), stdout.String())
		assert.Equal(t, drift, got.Drift)
		assert.Equal(t, "drift", got.Status)
		assert.NotEmpty(t, got.Error)
	})
}
