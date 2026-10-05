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
	}
	for _, tt := range tests {
		got := AnalyzerFor(tt.code)
		assert.Equal(t, AnalyzerInfo{tt.name, tt.scope}, got, tt.code)
	}
	SetAnalyzer("AR990", "custom", ScopeBundle)
	t.Cleanup(func() { delete(analyzerOverrides, "AR990") })
	assert.Equal(t, AnalyzerInfo{"custom", ScopeBundle}, AnalyzerFor("AR990"))
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
