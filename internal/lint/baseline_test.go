package lint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func finding(code, path string, line int, fp string) Finding {
	return Finding{Code: code, Name: code, Severity: SeverityError, File: path, Line: line, Message: "m " + fp, Meta: &FindingMeta{Path: path, Fingerprint: fp}}
}

func TestBaselineRoundTripAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", BaselineFile)
	b := &Baseline{Version: baselineVersion, Entries: []BaselineEntry{
		{Fingerprint: "ar1:b", Code: "AR401", File: "z.md", Reason: "legacy", Expires: "2027-01-31"},
		{Fingerprint: "ar1:a", Code: "AR201", File: "a.md"},
	}}
	require.NoError(t, b.Save(path))
	got, err := LoadBaseline(path)
	require.NoError(t, err)
	assert.Equal(t, "a.md", got.Entries[0].File, "entries are sorted by file for stable diffs")
	assert.Equal(t, "legacy", got.Entries[1].Reason)

	missing, err := LoadBaseline(filepath.Join(t.TempDir(), "none.json"))
	require.NoError(t, err)
	assert.Nil(t, missing)

	for name, body := range map[string]string{
		"bad json":    "{",
		"bad version": `{"version":9,"entries":[]}`,
		"no fp":       `{"version":1,"entries":[{"code":"AR1"}]}`,
		"bad date":    `{"version":1,"entries":[{"fingerprint":"x","expires":"next week"}]}`,
	} {
		p := filepath.Join(t.TempDir(), "b.json")
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
		_, err := LoadBaseline(p)
		assert.Error(t, err, name)
	}
}

func TestApplyBaseline(t *testing.T) {
	r := &Report{Findings: []Finding{
		finding(CodePathMissing, "a.md", 3, "fp-accepted"),
		finding(CodePathMissing, "a.md", 9, "fp-new"),
		finding(CodeLinkUnresolved, "b.md", 1, "fp-expired"),
	}}
	b := &Baseline{Version: 1, Entries: []BaselineEntry{
		{Fingerprint: "fp-accepted", Reason: "known"},
		{Fingerprint: "fp-expired", Expires: "2026-01-01"},
		{Fingerprint: "fp-gone"},
	}}

	res := ApplyBaseline(r, b, "b.json", "2026-10-05")

	assert.Equal(t, 1, res.Accepted)
	assert.True(t, r.Findings[0].IsAccepted())
	assert.Equal(t, "known", r.Findings[0].Meta.AcceptReason)
	assert.False(t, r.Findings[1].IsAccepted(), "a finding not in the baseline is new")
	assert.False(t, r.Findings[2].IsAccepted(), "an expired entry stops accepting")
	require.Len(t, res.Expired, 1)
	require.Len(t, res.Stale, 1)
	assert.Equal(t, "fp-gone", res.Stale[0].Fingerprint)

	// Expiry is inclusive: it still applies on its last day.
	r2 := &Report{Findings: []Finding{finding(CodePathMissing, "a.md", 1, "fp-expired")}}
	assert.Equal(t, 1, ApplyBaseline(r2, b, "b.json", "2026-01-01").Accepted)
}

func TestUpdateBaselineKeepsReasonsAndDropsStale(t *testing.T) {
	prev := &Baseline{Version: 1, Entries: []BaselineEntry{
		{Fingerprint: "keep", Reason: "legacy", Expires: "2027-01-01"},
		{Fingerprint: "stale", Reason: "old"},
	}}
	r := &Report{Findings: []Finding{
		finding(CodePathMissing, "a.md", 1, "keep"),
		finding(CodeLinkUnresolved, "b.md", 1, "fresh"),
	}}

	next, err := UpdateBaseline(r, prev, "accepted in bulk")

	require.NoError(t, err)
	require.Len(t, next.Entries, 2)
	byFP := map[string]BaselineEntry{}
	for _, e := range next.Entries {
		byFP[e.Fingerprint] = e
	}
	assert.Equal(t, "legacy", byFP["keep"].Reason)
	assert.Equal(t, "2027-01-01", byFP["keep"].Expires)
	assert.Equal(t, "accepted in bulk", byFP["fresh"].Reason)
	assert.NotContains(t, byFP, "stale")
}

func TestUpdateBaselineRequiresReasonForSecurityFindings(t *testing.T) {
	r := &Report{Findings: []Finding{finding(CodeSecretDetected, "a.md", 1, "s")}}
	_, err := UpdateBaseline(r, nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "needs a reason")
	_, err = UpdateBaseline(r, nil, "test fixture key")
	assert.NoError(t, err)
}

func TestBudgetsTolerateUpToTheCap(t *testing.T) {
	findings := []Finding{
		finding(CodePathMissing, "a.md", 1, "1"), finding(CodePathMissing, "a.md", 2, "2"),
		finding(CodeLinkUnresolved, "a.md", 3, "3"),
	}
	within := ResolveRatchet(map[string]int{"AR401": 2, "nonsense": 1})
	assert.Equal(t, Ratchet{CodePathMissing: 2}, within)
	assert.Empty(t, within.Excess(findings))
	assert.True(t, FailedWith(findings, "error", within), "AR201 is unbudgeted and still fails")
	assert.False(t, FailedWith(findings[:2], "error", within), "AR401 within budget is tolerated")

	tight := ResolveRatchet(map[string]int{"path-missing": 1})
	assert.Equal(t, []RatchetExcess{{Code: CodePathMissing, Count: 2, Max: 1}}, tight.Excess(findings))
	assert.True(t, FailedWith(findings[:2], "error", tight), "over budget: every finding of the rule counts again")

	findings[0].meta().Accepted = true
	assert.Empty(t, tight.Excess(findings), "accepted findings do not consume the budget")
}

func TestFailedIgnoresAcceptedFindings(t *testing.T) {
	f := finding(CodePathMissing, "a.md", 1, "x")
	assert.True(t, Failed([]Finding{f}, "error"))
	f.meta().Accepted = true
	assert.False(t, Failed([]Finding{f}, "error"))
}

func TestValidateSettingsRatchet(t *testing.T) {
	problems := ValidateSettings(&config.LintConfig{Ratchet: map[string]int{"AR401": 3, "AR999": 1, "AR201": -1}})
	assert.Equal(t, []string{"lint.ratchet.AR201: -1 is negative", `lint.ratchet: unknown rule "AR999"`}, problems)
}

func TestApplyBaselineRefusesPolicyProtectedCodes(t *testing.T) {
	// Arrange: AR001 is protected; the baseline accepts it and AR002-like finding.
	r := &Report{
		Root: "root", ConfigFile: ".ai-rulez/config.toml",
		Protected: map[string]bool{CodeSecretDetected: true},
		Findings: []Finding{
			finding(CodeSecretDetected, "a.md", 3, "fp-secret"),
			finding(CodePathMissing, "a.md", 9, "fp-path"),
		},
	}
	b := &Baseline{Version: 1, Entries: []BaselineEntry{{Fingerprint: "fp-secret"}, {Fingerprint: "fp-path"}}}

	// Act
	res := ApplyBaseline(r, b, "b.json", "2026-10-05")

	// Assert
	assert.Equal(t, 1, res.Accepted)
	assert.False(t, r.Findings[0].IsAccepted(), "a protected code is never accepted")
	assert.True(t, r.Findings[1].IsAccepted())
	assert.Empty(t, res.Stale, "the refused entry matched a finding")
	require.Len(t, r.Findings, 3)
	attempt := r.Findings[2]
	assert.Equal(t, CodePolicyLoosened, attempt.Code)
	assert.Equal(t, SeverityError, attempt.Severity)
	assert.Contains(t, attempt.Message, "baseline")
	assert.Contains(t, attempt.Message, CodeSecretDetected)
	assert.True(t, Failed(r.Findings, "error"))
}

func TestBudgetsWithoutDropsProtectedCodes(t *testing.T) {
	// Arrange
	b := Ratchet{CodeSecretDetected: 5, CodePathMissing: 2}
	findings := []Finding{finding(CodeSecretDetected, "a.md", 1, "x")}

	// Act
	kept, dropped := b.Without(map[string]bool{CodeSecretDetected: true})

	// Assert
	assert.Equal(t, Ratchet{CodePathMissing: 2}, kept)
	assert.Equal(t, []string{CodeSecretDetected}, dropped)
	assert.True(t, FailedWith(findings, "error", kept), "the protected finding counts against the exit code")
	same, none := b.Without(nil)
	assert.Equal(t, b, same)
	assert.Empty(t, none)
}

func TestNarrowedRunKeepsBaselineEntriesItDidNotLookFor(t *testing.T) {
	prev := &Baseline{Version: 1, Entries: []BaselineEntry{
		{Fingerprint: "other", Code: CodePathMissing, File: "a.md", Reason: "legacy", Expires: "2027-01-01"},
		{Fingerprint: "ext", Code: CodeExternalFinding, File: "a.md", Reason: "scanner"},
		{Fingerprint: "sec-gone", Code: CodeSecretDetected, File: "a.md", Reason: "fixture"},
	}}
	r := &Report{Analyzers: []string{AnalyzerSecurity}, Findings: []Finding{finding(CodeShellExec, "s.md", 1, "fresh")}}

	res := ApplyBaseline(r, prev, "b.json", "2026-10-05")
	require.Len(t, res.Stale, 1, "only the entry of a rule the run checked can be stale")
	assert.Equal(t, "sec-gone", res.Stale[0].Fingerprint)

	next, err := UpdateBaseline(r, prev, "why")
	require.NoError(t, err)
	byFP := map[string]BaselineEntry{}
	for _, e := range next.Entries {
		byFP[e.Fingerprint] = e
	}
	assert.Contains(t, byFP, "other", "a security-only run must not delete non-security entries")
	assert.Equal(t, "legacy", byFP["other"].Reason)
	assert.Equal(t, "2027-01-01", byFP["other"].Expires)
	assert.Contains(t, byFP, "ext", "a run without --external must keep scanner entries")
	assert.Contains(t, byFP, "fresh")
	assert.NotContains(t, byFP, "sec-gone", "stale entries of a checked rule are still pruned")
}
