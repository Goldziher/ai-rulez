package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEveryRegisteredRuleHasAnAnalyzerAndScope(t *testing.T) {
	for _, r := range Rules() {
		a := AnalyzerFor(r.Code)
		assert.NotEmpty(t, a.Name, r.Code)
		assert.Contains(t, []string{ScopeFile, ScopeItem, ScopeBundle}, a.Scope, r.Code)
	}
}

// A registered code that falls back on its family would silently land in the
// wrong analyzer (every AR9xx letter-family code used to become "budgets").
func TestEveryRegisteredCodeHasAnExplicitAnalyzer(t *testing.T) {
	for _, r := range Rules() {
		_, ok := analyzerOverrides[r.Code]
		assert.True(t, ok, "%s (%s) has no entry in analyzerGroups", r.Code, r.Name)
	}
	listed := map[string]string{}
	for _, g := range analyzerGroups {
		for _, code := range g.codes {
			prev, dup := listed[code]
			assert.False(t, dup, "%s is listed under %s and %s", code, prev, g.name)
			listed[code] = g.name
			_, registered := lookupRule(code)
			assert.True(t, registered, "%s is classified but not registered", code)
		}
	}
}

func TestAnalyzerClassification(t *testing.T) {
	tests := []struct {
		code, name, scope string
	}{
		{CodeSecretDetected, AnalyzerSecurity, ScopeFile},
		{CodeUnpinnedRemote, AnalyzerSecurity, ScopeBundle},
		{CodeLinkUnresolved, AnalyzerReferences, ScopeFile},
		{CodeHookNotExecutable, AnalyzerHooks, ScopeBundle},
		{CodeDescriptionDup, AnalyzerDuplicates, ScopeBundle},
		{CodeDescriptionMissing, AnalyzerDescriptions, ScopeItem},
		{CodeSizeLines, AnalyzerBudgets, ScopeItem},
		{CodeMetadataMissing, AnalyzerMetadata, ScopeItem},
		{CodePluginVersionDrift, AnalyzerPlugin, ScopeBundle},
		// Codes a later rule package adds fall back on their family.
		{"AR013", AnalyzerSecurity, ScopeItem},
		{"AR403", AnalyzerReferences, ScopeItem},
		{"AR964", AnalyzerPlugin, ScopeItem},
		{"AR805", AnalyzerDescriptions, ScopeItem},
		{"AR9E1", AnalyzerSecurity, ScopeBundle},
		{"AR9L1", AnalyzerSecurity, ScopeBundle},
		{"AR9K1", AnalyzerSecurity, ScopeBundle},
		{"AR9K0", AnalyzerConfig, ScopeBundle},
		{"AR9C2", AnalyzerTraps, ScopeFile},
		{"AR995", AnalyzerLock, ScopeBundle},
		{"AR981", AnalyzerLock, ScopeBundle},
		{"AR9B4", AnalyzerOKF, ScopeItem},
		{"AR9A0", AnalyzerEvals, ScopeItem},
		{"AR9F1", AnalyzerConvert, ScopeItem},
	}
	for _, tt := range tests {
		got := AnalyzerFor(tt.code)
		assert.Equal(t, AnalyzerInfo{tt.name, tt.scope}, got, tt.code)
	}
	SetAnalyzer("AR999", "custom", ScopeBundle)
	t.Cleanup(func() { delete(analyzerOverrides, "AR999") })
	assert.Equal(t, AnalyzerInfo{"custom", ScopeBundle}, AnalyzerFor("AR999"))
}

func TestFilterAnalyzersAndFindingAnnotations(t *testing.T) {
	r := &Report{Findings: []Finding{
		finding(CodeSecretDetected, "a.md", 1, "1"), finding(CodeLinkUnresolved, "a.md", 2, "2"), finding(CodeSizeLines, "a.md", 3, "3"),
	}}
	for i := range r.Findings {
		annotateAnalyzer(&r.Findings[i])
	}
	assert.Equal(t, AnalyzerSecurity, r.Findings[0].Meta.Analyzer)
	assert.Equal(t, ScopeFile, r.Findings[0].Meta.Scope)

	FilterAnalyzers(r, []string{"Security", " budgets "})
	assert.Len(t, r.Findings, 2)
	FilterAnalyzers(r, nil)
	assert.Len(t, r.Findings, 2, "no names keeps everything")
	FilterAnalyzers(r, []string{"hooks"})
	assert.Empty(t, r.Findings)
}

func TestAnalyzerNamesAreSortedAndUnique(t *testing.T) {
	names := AnalyzerNames()
	assert.Contains(t, names, AnalyzerSecurity)
	assert.IsIncreasing(t, names)
}
