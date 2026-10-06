package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const trackedGolden = "../../tests/review/golden/skill-quality"

func calibrateProject(t *testing.T) *fakeModel {
	t.Helper()
	golden, err := filepath.Abs(trackedGolden)
	require.NoError(t, err)
	judgedProject(t, "")
	reviewFlags.semantic = false
	calibrateFlags.golden, calibrateFlags.content, calibrateFlags.format = golden, "full", "json"
	calibrateFlags.noProbes, calibrateFlags.noWrite, calibrateFlags.compare, calibrateFlags.models, calibrateFlags.model, calibrateFlags.out = true, false, "", "", "", ""
	require.NoError(t, reviewCalibrateCmd.Flags().Set("max-cost", "0"))
	t.Cleanup(func() {
		calibrateFlags.golden, calibrateFlags.noProbes, calibrateFlags.compare, calibrateFlags.format = "", false, "", formatText
		reviewCalibrateCmd.Flags().Lookup("max-cost").Changed = false
	})
	f := &fakeModel{} // a judge that passes everything
	useFakeModel(t, f)
	return f
}

func TestReviewCalibrateWritesTheRecordAndFailsAJudgeThatMissesDefects(t *testing.T) {
	// Arrange
	calibrateProject(t)
	var out bytes.Buffer

	// Act
	exit, err := runCalibrate(reviewCalibrateCmd, &out)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, exitReviewGate, exit, "a judge that passes everything does not meet its thresholds")
	var got struct {
		Record struct {
			Status string
			NItems int `json:"n_items"`
			Model  string
		}
		RecordFile string `json:"record_file"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	assert.Equal(t, "fail", got.Record.Status)
	assert.Equal(t, 55, got.Record.NItems)
	assert.Equal(t, "fake/model", got.Record.Model)
	assert.Equal(t, filepath.Join("calibration", "skill-quality.builtin.json"), filepath.Join("calibration", filepath.Base(got.RecordFile)))
	assert.Equal(t, "calibration", filepath.Base(filepath.Dir(got.RecordFile)))
	_, statErr := os.Stat(got.RecordFile)
	require.NoError(t, statErr)
}

func TestReviewCalibrateCompareReportsDrift(t *testing.T) {
	// Arrange: calibrate once, then compare a second run with the record
	calibrateProject(t)
	_, err := runCalibrate(reviewCalibrateCmd, &bytes.Buffer{})
	require.NoError(t, err)
	record := filepath.Join(".ai-rulez", "calibration", "skill-quality.builtin.json")
	calibrateFlags.compare = record
	var out bytes.Buffer

	// Act
	exit, err := runCalibrate(reviewCalibrateCmd, &out)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, exitReviewGate, exit)
	assert.Contains(t, out.String(), "no longer meets the calibration thresholds")
	assert.Contains(t, out.String(), `"drift"`)
}

func TestReviewCalibrateNeedsAGoldenSetForABuiltinRubric(t *testing.T) {
	// Arrange
	calibrateProject(t)
	calibrateFlags.golden = ""

	// Act
	_, err := runCalibrate(reviewCalibrateCmd, &bytes.Buffer{})

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no golden set of its own")
}

func TestReviewCalibrateIsRefusedWithoutTheNetworkOptIn(t *testing.T) {
	// Arrange
	f := calibrateProject(t)
	t.Setenv("AI_RULEZ_LLM_ALLOW_NETWORK", "")

	// Act
	_, err := runCalibrate(reviewCalibrateCmd, &bytes.Buffer{})

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LLM access is disabled")
	assert.Zero(t, f.calls)
}

func TestReviewCalibrateModelsComparesWithoutWriting(t *testing.T) {
	// Arrange
	calibrateProject(t)
	calibrateFlags.models = "model,other-model"
	require.NoError(t, reviewCalibrateCmd.Flags().Set("max-calls", "5000"))
	t.Cleanup(func() { reviewCalibrateCmd.Flags().Lookup("max-calls").Changed = false })
	var out bytes.Buffer

	// Act
	_, err := runCalibrate(reviewCalibrateCmd, &out)

	// Assert
	require.NoError(t, err)
	assert.Contains(t, out.String(), `"comparison"`)
	_, statErr := os.Stat(filepath.Join(".ai-rulez", "calibration"))
	assert.True(t, os.IsNotExist(statErr), "a comparison writes no record")
}
