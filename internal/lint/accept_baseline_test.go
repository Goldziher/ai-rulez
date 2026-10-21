package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configAccepted(code, fp string) Finding {
	return Finding{Code: code, File: "a.md", Line: 1, Message: "m", Meta: &FindingMeta{
		Fingerprint: fp, Accepted: true, AcceptedByConfig: true, AcceptReason: "from config",
	}}
}

// A finding that [[lint.accept]] accepted keeps its reason, is not counted as a
// baseline acceptance, and still makes its baseline entry count as matched.
func TestApplyBaselineLeavesConfigAcceptedFindingsAlone(t *testing.T) {
	rep := &Report{Findings: []Finding{
		configAccepted("AR401", "ar1:cfg"),
		{Code: "AR401", File: "b.md", Line: 1, Meta: &FindingMeta{Fingerprint: "ar1:base"}},
	}}
	b := &Baseline{Version: baselineVersion, Entries: []BaselineEntry{
		{Fingerprint: "ar1:cfg", Code: "AR401", File: "a.md", Reason: "baseline reason"},
		{Fingerprint: "ar1:base", Code: "AR401", File: "b.md", Reason: "baseline reason"},
	}}

	res := ApplyBaseline(rep, b, "lint-baseline.json", "2030-01-01")

	assert.Equal(t, 1, res.Accepted, "only the baseline's own acceptance counts")
	assert.Empty(t, res.Stale, "the entry matching a config-accepted finding is not stale")
	assert.Equal(t, "from config", rep.Findings[0].Meta.AcceptReason)
	assert.Equal(t, "baseline reason", rep.Findings[1].Meta.AcceptReason)
	assert.True(t, rep.Findings[1].IsAccepted())
}

func TestUpdateBaselineSkipsConfigAcceptedFindings(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{"security code needs no baseline reason", "AR001"},
		{"ordinary code", "AR401"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: one finding accepted by config, one accepted by the previous baseline.
			baselined := Finding{Code: "AR401", File: "b.md", Line: 1, Message: "m",
				Meta: &FindingMeta{Fingerprint: "ar1:base", Accepted: true, AcceptReason: "kept"}}
			rep := &Report{Findings: []Finding{configAccepted(tt.code, "ar1:cfg"), baselined}}
			prev := &Baseline{Version: baselineVersion, Entries: []BaselineEntry{
				{Fingerprint: "ar1:base", Code: "AR401", File: "b.md", Reason: "kept"},
			}}

			// Act
			got, err := UpdateBaseline(rep, prev, "")

			// Assert
			require.NoError(t, err)
			require.Len(t, got.Entries, 1)
			assert.Equal(t, "ar1:base", got.Entries[0].Fingerprint)
			assert.Equal(t, "kept", got.Entries[0].Reason)
		})
	}
}
