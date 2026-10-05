package lint

import (
	"bytes"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sev(code, path string, s Severity) Finding {
	f := finding(code, path, 1, code+path)
	f.Severity = s
	return f
}

func TestRiskScoreAndLabel(t *testing.T) {
	w := DefaultRiskWeights()
	tests := []struct {
		name  string
		in    []Finding
		score int
		label string
	}{
		{"clean", nil, 0, RiskClean},
		{"one info", []Finding{sev("AR202", "a", SeverityInfo)}, 1, RiskLow},
		{"one warning floors at medium", []Finding{sev("AR401", "a", SeverityWarning)}, 8, RiskMedium},
		{"one error floors at high despite a low score", []Finding{sev("AR201", "a", SeverityError)}, 25, RiskHigh},
		{"many warnings by score", []Finding{
			sev("AR401", "a", SeverityWarning), sev("AR401", "a", SeverityWarning), sev("AR401", "a", SeverityWarning),
			sev("AR401", "a", SeverityWarning), sev("AR401", "a", SeverityWarning), sev("AR401", "a", SeverityWarning),
			sev("AR401", "a", SeverityWarning), sev("AR401", "a", SeverityWarning), sev("AR401", "a", SeverityWarning),
			sev("AR401", "a", SeverityWarning),
		}, 80, RiskCritical},
		{"capped at 100", []Finding{
			sev("AR201", "a", SeverityError), sev("AR201", "a", SeverityError), sev("AR201", "a", SeverityError),
			sev("AR201", "a", SeverityError), sev("AR201", "a", SeverityError),
		}, 100, RiskCritical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeRisk(tt.in, w).Bundle
			assert.Equal(t, tt.score, got.Score)
			assert.Equal(t, tt.label, got.Label)
		})
	}
}

func TestRiskIsPerItemAndSkipsAccepted(t *testing.T) {
	accepted := sev("AR201", "b.md", SeverityError)
	accepted.meta().Accepted = true
	got := ComputeRisk([]Finding{
		sev("AR401", "a.md", SeverityWarning), sev("AR401", "a.md", SeverityWarning),
		sev("AR202", "c.md", SeverityInfo), accepted,
	}, DefaultRiskWeights())

	require.Len(t, got.Items, 2, "an item with only accepted findings carries no risk")
	assert.Equal(t, "a.md", got.Items[0].Item, "riskiest first")
	assert.Equal(t, 16, got.Items[0].Score)
	assert.Equal(t, RiskMedium, got.Items[0].Label)
	assert.Equal(t, RiskLow, got.Items[1].Label)
	assert.Equal(t, 17, got.Bundle.Score)
	assert.Equal(t, 3, got.Bundle.Findings)
}

func TestRiskWeightsAreConfigurable(t *testing.T) {
	zero, big := 0, 60
	w := RiskWeightsFrom(&config.LintRisk{Error: &big, Info: &zero})
	assert.Equal(t, RiskWeights{Error: 60, Warning: 8, Info: 0}, w)
	got := ComputeRisk([]Finding{sev("AR201", "a", SeverityError), sev("AR202", "a", SeverityInfo)}, w).Bundle
	assert.Equal(t, 60, got.Score)
	assert.Equal(t, RiskHigh, got.Label)
	assert.Equal(t, DefaultRiskWeights(), RiskWeightsFrom(nil))

	neg := -1
	assert.Equal(t, []string{"lint.risk.warning: -1 is negative"}, ValidateSettings(&config.LintConfig{Risk: &config.LintRisk{Warning: &neg}}))
}

func TestRiskNeverChangesFailure(t *testing.T) {
	// Blocking stays threshold-by-severity: a tiny risk score still fails on an error.
	zero := 0
	fs := []Finding{sev("AR201", "a", SeverityError)}
	risk := ComputeRisk(fs, RiskWeightsFrom(&config.LintRisk{Error: &zero}))
	assert.Equal(t, RiskHigh, risk.Bundle.Label, "the floor still applies")
	assert.True(t, Failed(fs, "error"))
}

func TestRiskShownInEveryFormat(t *testing.T) {
	r := &Report{Root: ".", Findings: sampleCombined().Findings}
	rr := ComputeRisk(r.Findings, DefaultRiskWeights())
	r.Risk = &rr
	c := Combine([]*Report{r})

	var buf bytes.Buffer
	require.NoError(t, WriteText(&buf, c))
	assert.Contains(t, buf.String(), "risk (advisory, never fails the run): high (score 34/100)")
	assert.Contains(t, buf.String(), ".ai-rulez/rules/a b.md")

	assert.Contains(t, string(render(t, FormatMarkdown, c, WriteOptions{})), "Risk (advisory, does not block): **high (score 34/100)**")
	assert.Contains(t, string(render(t, FormatJSON, c, WriteOptions{})), `"label": "high"`)
	sarif := render(t, FormatSARIF, c, WriteOptions{})
	validateSARIF(t, sarif)
	assert.Contains(t, string(sarif), `"risk"`)
}

func TestCombinedRiskSumsRootsAndCaps(t *testing.T) {
	mk := func(root string, n int) *Report {
		var fs []Finding
		for i := 0; i < n; i++ {
			fs = append(fs, sev("AR201", root+"/f.md", SeverityError))
		}
		rr := ComputeRisk(fs, DefaultRiskWeights())
		return &Report{Root: root, Findings: fs, Risk: &rr}
	}
	c := Combine([]*Report{mk("a", 2), mk("b", 3)})
	require.NotNil(t, c.Risk)
	assert.Equal(t, 100, c.Risk.Bundle.Score, "50 + 75 capped")
	assert.Len(t, c.Risk.Roots, 2)
	assert.Nil(t, Combine([]*Report{{Root: "x"}}).Risk, "no risk computed, none reported")
}
