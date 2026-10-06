package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetCalibrateFlags(t *testing.T) {
	t.Helper()
	reset := func() {
		evalCalibrateFlags.results, evalCalibrateFlags.harness, evalCalibrateFlags.model = "", "", ""
		evalCalibrateFlags.minSamples, evalCalibrateFlags.format = evals.DefaultMinSamples, formatText
	}
	reset()
	t.Cleanup(reset)
}

// recordNativeRun leaves a native activation record (with the token split) in the
// project's results file by running the fake claude.
func recordNativeRun(t *testing.T) {
	t.Helper()
	resetEvalFlags(t)
	activationProject(t)
	evalFlags.mode, evalFlags.surface, evalFlags.runs = evals.ModeActivation, evals.SurfaceNative, 2
	evalFlags.claudeBin, evalFlags.format, evalFlags.date, evalFlags.model = fakeClaude(t), evals.FormatJSON, "2026-10-06", "haiku"
	evalRunCmd.SetOut(&bytes.Buffer{})
	failed, err := runEval(evalRunCmd, []string{"deploy"})
	require.NoError(t, err)
	require.False(t, failed)
}

func TestEvalCalibrateEstimate_ProposesFromTheRecordedRun(t *testing.T) {
	// Arrange: the fake claude reports 100 input and 10 output tokens per run.
	recordNativeRun(t)
	resetCalibrateFlags(t)
	var out bytes.Buffer
	evalCalibrateCmd.SetOut(&out)

	// Act
	err := runEvalCalibrate(evalCalibrateCmd)

	// Assert
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "activation, harness claude, model haiku: 1 run(s) (low confidence")
	assert.Contains(t, text, "[lint.evals.estimate]")
	assert.Contains(t, text, "overhead_tokens = ")
	assert.Contains(t, text, "activation_output_tokens = 10")
}

func TestEvalCalibrateEstimate_JSONAndFilters(t *testing.T) {
	recordNativeRun(t)
	resetCalibrateFlags(t)
	evalCalibrateFlags.format = formatJSON
	var out bytes.Buffer
	evalCalibrateCmd.SetOut(&out)

	require.NoError(t, runEvalCalibrate(evalCalibrateCmd))

	var cal evals.Calibration
	require.NoError(t, json.Unmarshal(out.Bytes(), &cal))
	require.Len(t, cal.Groups, 1)
	assert.Equal(t, 10, cal.Groups[0].Proposed.ActivationOutputTokens)

	evalCalibrateFlags.model = "opus"
	out.Reset()
	require.NoError(t, runEvalCalibrate(evalCalibrateCmd))
	require.NoError(t, json.Unmarshal(out.Bytes(), &cal))
	assert.Empty(t, cal.Groups)
	assert.Equal(t, 1, cal.Skipped.Filtered)
}

func TestEvalCalibrateEstimate_NoResultsIsNotAnError(t *testing.T) {
	resetEvalFlags(t)
	evalProject(t)
	resetCalibrateFlags(t)
	var out bytes.Buffer
	evalCalibrateCmd.SetOut(&out)

	require.NoError(t, runEvalCalibrate(evalCalibrateCmd))

	assert.Contains(t, out.String(), "No recorded run has a token split")
}

func TestEvalCalibrateEstimate_Errors(t *testing.T) {
	tests := []struct {
		name    string
		set     func()
		wantErr string
	}{
		{"negative min samples", func() { evalCalibrateFlags.minSamples = -1 }, "--min-samples"},
		{"unknown format", func() { evalCalibrateFlags.format = "xml" }, "unknown --format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetEvalFlags(t)
			evalProject(t)
			resetCalibrateFlags(t)
			tt.set()

			err := runEvalCalibrate(evalCalibrateCmd)

			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestEvalRun_AppliesTheConfiguredEstimateAssumptions(t *testing.T) {
	resetEvalFlags(t)
	root := activationProject(t)
	cfg := filepath.Join(root, ".ai-rulez", "config.toml")
	body := "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n\n[lint.evals.estimate]\noverhead_tokens = 30000\n"
	require.NoError(t, os.WriteFile(cfg, []byte(body), 0o600))
	evalFlags.mode, evalFlags.surface, evalFlags.estimate, evalFlags.format = evals.ModeActivation, evals.SurfaceNative, true, evals.FormatJSON
	var out bytes.Buffer
	evalRunCmd.SetOut(&out)

	_, err := runEval(evalRunCmd, []string{"deploy"})

	require.NoError(t, err)
	var report evals.ActivationReport
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	require.NotNil(t, report.Estimate)
	assert.Greater(t, report.Estimate.InputTokens, report.Estimate.AgentRuns*30000, "the configured overhead is in the estimate")
}
