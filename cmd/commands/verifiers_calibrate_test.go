package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/verifiers"
)

const calibrateVerifierFile = "[[verifiers]]\nid = \"calibrated\"\nrule = \"r\"\nseverity = \"error\"\nwhen_changed = [\"*.txt\"]\n" +
	"[verifiers.require.llm]\nchecklist = [\"Errors say what to do.\"]\n" +
	"[[verifiers.examples]]\nname = \"bad\"\nchanged = [\"a.txt\"]\nexpect = \"fail\"\n[verifiers.examples.files]\n\"a.txt\" = \"error: it broke\\n\"\n"

func resetVerifierCalibrateFlags(t *testing.T) {
	t.Helper()
	resetVerifiersFlags(t)
	reset := func() { calibrateNoWrite, calibrateJSON, verifiersGateLLM = false, false, false }
	reset()
	t.Cleanup(reset)
}

func verifierCalibrateProject(t *testing.T) string {
	t.Helper()
	verifiersLLMProject(t, "\n[llm]\nprovider = \"gemini\"\nmodel = \"gemini-2.5-flash\"\n")
	root, err := os.Getwd()
	require.NoError(t, err)
	writeFile(t, filepath.Join(root, ".ai-rulez", "verifiers", "llm.toml"), calibrateVerifierFile)
	return root
}

func TestCalibrateVerifiers(t *testing.T) {
	tests := []struct {
		name     string
		setup    func()
		args     []string
		wantCode int
		want     string
	}{
		{"without --allow-llm there is nothing to measure with", func() {}, nil, exitVerifiersCannotRun, ""},
		{"estimate calls nothing and prints the manifest", func() { verifiersEstimate = true }, nil, 0, "estimate: 1 call(s) to gemini/gemini-2.5-flash"},
		{"an unknown verifier", func() { verifiersEstimate = true }, []string{"ghost"}, exitVerifiersCannotRun, ""},
		{"no-write and estimate conflict", func() { verifiersEstimate, calibrateNoWrite = true, true }, nil, exitVerifiersCannotRun, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			resetVerifierCalibrateFlags(t)
			root := verifierCalibrateProject(t)
			tt.setup()
			var out bytes.Buffer

			// Act
			code := calibrateVerifiers(context.Background(), tt.args, &out)

			// Assert
			assert.Equal(t, tt.wantCode, code, out.String())
			if tt.want != "" {
				assert.Contains(t, out.String(), tt.want)
			}
			entries, _ := os.ReadDir(filepath.Join(root, ".ai-rulez", "verifiers", "calibration"))
			assert.Empty(t, entries, "nothing was measured, so nothing is written")
		})
	}
}

func TestRenderCalibration_ListsMissesAndTheRecordHint(t *testing.T) {
	rep := &verifiers.CalibrationReport{Records: []verifiers.Calibration{{
		Verifier: "actionable", Status: verifiers.CalibrationFail, NItems: 4, Model: "m1", Date: "2026-10-07",
		Precision: 0.5, PrecisionCI: [2]float64{0.1, 0.9}, Recall: 1, RecallCI: [2]float64{0.3, 1}, TP: 1, FP: 1, TN: 1, FN: 0,
		Misses: []string{"precision 0.50 is below 0.80 (1 of 2 flagged examples were real failures)"},
	}}}
	out := renderCalibration(rep, true)

	assert.Contains(t, out, "actionable: FAIL on 4 examples with m1 (2026-10-07)")
	assert.Contains(t, out, "precision 0.50 (95% interval 0.10-0.90)")
	assert.Contains(t, out, "- precision 0.50 is below 0.80")
	assert.True(t, strings.HasSuffix(out, "(commit them).\n"))
}
