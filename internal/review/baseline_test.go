package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

func TestBaselineHidesKnownFindingsAndReportsNewOnes(t *testing.T) {
	// Arrange: a first judged run, written as a baseline
	rb := builtin(t)
	run := func(descB string) (*Results, *Report) {
		res := Run(Input{Rubric: rb, Items: []Item{skill("a", "Helps with deployments"), skill("b", descB)}})
		sj := &scriptedJudge{verdict: func(_, dim string, _ int) string { return pick(dim == "trigger-quality", VerdictFail) }}
		client, _ := newClient(t, sj, llm.Config{})
		mustRun(t, SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: 1}})
		return res, NewReport(rb, res, nil)
	}
	_, first := run("Helps with releases")
	path := filepath.Join(t.TempDir(), "baseline.json")
	require.NoError(t, WriteBaseline(path, first))

	// Act: the same project again, plus a second run where item b's finding changed
	base, err := LoadBaseline(path)
	require.NoError(t, err)
	res2, _ := run("Helps with releases")
	res2.SetBaseline(base.Set())
	again := NewReport(rb, res2, nil)
	res3, _ := run("Does everything for releases")
	res3.SetBaseline(base.Set())
	changed := NewReport(rb, res3, nil)

	// Assert
	for _, f := range again.Findings {
		if f.Origin == OriginLLMJudge {
			assert.True(t, f.Baselined, "%s is already accepted", f.ItemID)
		}
	}
	hidden, fresh := res2.BaselineCounts(rb)
	assert.Equal(t, 2, hidden)
	assert.Zero(t, fresh)
	_, freshChanged := res3.BaselineCounts(rb)
	assert.Equal(t, 1, freshChanged, "a finding on changed text has another quote: it is new")
	_ = changed
}

func TestLoadBaselineAcceptsAReviewReport(t *testing.T) {
	// Arrange
	rb := builtin(t)
	res := Run(Input{Rubric: rb, Items: []Item{skill("a", "Helps with deployments")}})
	sj := &scriptedJudge{verdict: func(_, dim string, _ int) string { return pick(dim == "trigger-quality", VerdictWarn) }}
	client, _ := newClient(t, sj, llm.Config{})
	mustRun(t, SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: 1}})
	res.AddNote(Finding{Code: "AR9G9", Name: "judge-calibration-stale", Severity: "info", Path: "p", Line: 1, Message: "m", Origin: OriginRun, Fingerprint: "run-calibration"})
	rep := NewReport(rb, res, nil)
	raw, err := json.Marshal(rep)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "report.json")
	require.NoError(t, os.WriteFile(path, raw, 0o600))

	// Act
	b, err := LoadBaseline(path)

	// Assert
	require.NoError(t, err)
	assert.Len(t, b.Fingerprints, 1, "the judged finding, not the run notes")
	other := filepath.Join(t.TempDir(), "other.json")
	require.NoError(t, os.WriteFile(other, []byte(`{"schema":"something-else/1"}`), 0o600))
	_, err = LoadBaseline(other)
	require.Error(t, err)
}
