package verifiers

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// calibSpec is an llm verifier with n labelled examples per class: a file with a
// "BAD" line must fail, one without must pass.
func calibSpec(checklist string, failing, passing int) string {
	var b strings.Builder
	b.WriteString("[[verifiers]]\nid = \"no-bad\"\nrule = \"database\"\nseverity = \"error\"\nwhen_changed = [\"*.go\"]\n")
	fmt.Fprintf(&b, "[verifiers.require.llm]\nchecklist = [%q]\n", checklist)
	for i := 0; i < failing; i++ {
		fmt.Fprintf(&b, "[[verifiers.examples]]\nname = \"bad %d\"\nchanged = [\"bad%d.go\"]\nexpect = \"fail\"\n[verifiers.examples.files]\n\"bad%d.go\" = \"package a\\n// BAD thing %d\\n\"\n", i, i, i, i)
	}
	for i := 0; i < passing; i++ {
		fmt.Fprintf(&b, "[[verifiers.examples]]\nname = \"good %d\"\nchanged = [\"good%d.go\"]\nexpect = \"pass\"\n[verifiers.examples.files]\n\"good%d.go\" = \"package a\\n// fine %d\\n\"\n", i, i, i, i)
	}
	return b.String()
}

// badFlagger answers like a model that flags every added line containing needle.
func badFlagger(needle string) *llm.Fake {
	return &llm.Fake{ChatFunc: func(req llm.ChatRequest) (string, error) {
		user := req.Messages[1].Content
		file := ""
		for _, line := range strings.Split(user, "\n") {
			if rest, ok := strings.CutPrefix(line, "FILE "); ok {
				file = rest
			}
			if strings.Contains(line, needle) && strings.Contains(line, "+ ") {
				_, quote, _ := strings.Cut(line, "+ ")
				return reply([5]any{1, "fail", file, quote, "it says so"}), nil
			}
		}
		return reply([5]any{1, "pass", "", "", ""}), nil
	}}
}

var calibNow = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

func calibrateOnce(t *testing.T, cfg *config.Config, client llm.Client) *CalibrationReport {
	t.Helper()
	rep, err := Calibrate(context.Background(), cfg, CalibrateOptions{LLM: LLMOptions{Client: client, Model: "m1"}, Now: calibNow})
	require.NoError(t, err)
	return rep
}

func TestCalibrate_RecordsPrecisionAndRecall(t *testing.T) {
	tests := []struct {
		name          string
		client        llm.Client
		failing       int
		passing       int
		wantStatus    string
		wantPrecision float64
		wantRecall    float64
		wantMiss      string
	}{
		{"a model that is right", badFlagger("BAD"), 5, 5, CalibrationPass, 1, 1, ""},
		{"a model that flags everything is imprecise", badFlagger(""), 5, 5, CalibrationFail, 0.5, 1, "precision"},
		{"a model that flags nothing misses everything", badFlagger("NEVER-THERE"), 5, 5, CalibrationFail, 0, 0, "precision"},
		{"too few examples", badFlagger("BAD"), 2, 2, CalibrationFail, 1, 1, "at least"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := specProject(t, map[string]string{"a.go": "package a\n"}, calibSpec("No line says BAD.", tt.failing, tt.passing))

			// Act
			rep := calibrateOnce(t, cfg, tt.client)

			// Assert
			require.Len(t, rep.Records, 1)
			rec := rep.Records[0]
			assert.Equal(t, tt.wantStatus, rec.Status)
			assert.Equal(t, "no-bad", rec.Verifier)
			assert.Equal(t, "m1", rec.Model)
			assert.Equal(t, "2026-10-07", rec.Date)
			assert.Equal(t, tt.failing+tt.passing, rec.NItems)
			assert.InDelta(t, tt.wantPrecision, rec.Precision, 1e-9)
			assert.InDelta(t, tt.wantRecall, rec.Recall, 1e-9)
			if tt.wantMiss != "" {
				assert.Contains(t, strings.Join(rec.Misses, " "), tt.wantMiss)
			}
		})
	}
}

func TestCalibrate_AnExampleThatCannotBeEvaluatedFailsTheRecord(t *testing.T) {
	// Arrange: the model answers with something that is not the requested JSON.
	cfg := specProject(t, map[string]string{"a.go": "package a\n"}, calibSpec("No line says BAD.", 5, 5))
	broken := &llm.Fake{ChatFunc: func(llm.ChatRequest) (string, error) { return "not json", nil }}

	// Act
	rep := calibrateOnce(t, cfg, broken)

	// Assert
	require.Len(t, rep.Records, 1)
	assert.Equal(t, CalibrationFail, rep.Records[0].Status)
	assert.Equal(t, 10, rep.Records[0].Errors)
	assert.Contains(t, strings.Join(rep.Records[0].Misses, " "), "could not be evaluated")
}

func TestCalibrate_RefusesAVerifierThatIsNotLLMOrUnknown(t *testing.T) {
	cfg := specProject(t, map[string]string{"a.go": "x"}, importedRegex)
	cfg.Content.ImportedVerifiers = nil
	cfg.Verifiers = nil

	_, err := Calibrate(context.Background(), cfg, CalibrateOptions{Names: []string{"nope"}, LLM: LLMOptions{Client: badFlagger("BAD")}, Now: calibNow})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown")
}

func TestCalibrate_NeedsAModel(t *testing.T) {
	cfg := specProject(t, map[string]string{"a.go": "package a\n"}, calibSpec("No line says BAD.", 5, 5))

	_, err := Calibrate(context.Background(), cfg, CalibrateOptions{LLM: LLMOptions{Disabled: "pass --allow-llm"}, Now: calibNow})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pass --allow-llm")
}

func gateRun(t *testing.T, cfg *config.Config, gate bool, client llm.Client) Result {
	t.Helper()
	rep := Run(context.Background(), cfg, Options{LLM: &LLMOptions{Client: client, Model: "m1", Gate: gate}})
	require.NoError(t, rep.Err)
	require.NotEmpty(t, rep.Results)
	return rep.Results[0]
}

func TestGate_AnLLMVerdictGatesOnlyWhenCalibrated(t *testing.T) {
	const checklist = "No line says BAD."
	spec := calibSpec(checklist, 5, 5)
	tests := []struct {
		name         string
		prepare      func(t *testing.T, cfg *config.Config)
		gate         bool
		wantSeverity string
		wantAdvisory bool
		wantNote     string
	}{
		{"calibrated and --gate-llm", func(t *testing.T, cfg *config.Config) {
			rep := calibrateOnce(t, cfg, badFlagger("BAD"))
			require.NoError(t, SaveCalibration(cfg, &rep.Records[0]))
		}, true, "error", false, "gated"},
		{"calibrated but no --gate-llm", func(t *testing.T, cfg *config.Config) {
			rep := calibrateOnce(t, cfg, badFlagger("BAD"))
			require.NoError(t, SaveCalibration(cfg, &rep.Records[0]))
		}, false, "warning", true, "--gate-llm"},
		{"no record", func(*testing.T, *config.Config) {}, true, "warning", true, "not calibrated"},
		{"a record that failed its bar", func(t *testing.T, cfg *config.Config) {
			rep := calibrateOnce(t, cfg, badFlagger(""))
			require.NoError(t, SaveCalibration(cfg, &rep.Records[0]))
		}, true, "warning", true, "precision"},
		{"a record for another checklist", func(t *testing.T, cfg *config.Config) {
			other := specProject(t, map[string]string{"a.go": "package a\n"}, calibSpec("Something else entirely.", 5, 5))
			rep := calibrateOnce(t, other, badFlagger("BAD"))
			rec := rep.Records[0]
			require.NoError(t, SaveCalibration(cfg, &rec))
		}, true, "warning", true, "changed since"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: the changed file has a BAD line, so the verifier fails.
			cfg := specProject(t, map[string]string{"a.go": "package a\n// BAD thing\n"}, spec)
			tt.prepare(t, cfg)

			// Act
			res := gateRun(t, cfg, tt.gate, badFlagger("BAD"))

			// Assert
			require.Equal(t, StatusFail, res.Status, res.Message)
			assert.Equal(t, tt.wantSeverity, res.Severity)
			assert.Equal(t, tt.wantAdvisory, res.Advisory)
			assert.Contains(t, strings.Join(res.Notes, "\n"), tt.wantNote)
		})
	}
}

func TestGate_AStaleModelDoesNotGate(t *testing.T) {
	// Arrange: calibrated with m1, run with m2.
	cfg := specProject(t, map[string]string{"a.go": "package a\n// BAD thing\n"}, calibSpec("No line says BAD.", 5, 5))
	rep := calibrateOnce(t, cfg, badFlagger("BAD"))
	require.NoError(t, SaveCalibration(cfg, &rep.Records[0]))

	// Act
	run := Run(context.Background(), cfg, Options{LLM: &LLMOptions{Client: badFlagger("BAD"), Model: "m2", Gate: true}})

	// Assert
	require.NoError(t, run.Err)
	res := run.Results[0]
	assert.Equal(t, "warning", res.Severity)
	assert.Contains(t, strings.Join(res.Notes, "\n"), "m1")
}

func TestSaveCalibration_RoundTrips(t *testing.T) {
	cfg := specProject(t, map[string]string{"a.go": "package a\n"}, calibSpec("No line says BAD.", 5, 5))
	rep := calibrateOnce(t, cfg, badFlagger("BAD"))

	require.NoError(t, SaveCalibration(cfg, &rep.Records[0]))
	got, err := LoadCalibration(cfg, "no-bad")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, rep.Records[0], *got)
	missing, err := LoadCalibration(cfg, "other")
	require.NoError(t, err)
	assert.Nil(t, missing)
}
