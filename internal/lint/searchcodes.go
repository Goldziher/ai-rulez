package lint

// Codes of `ai-rulez search --eval` (internal/skillsearch). They are registered
// here so `validate --explain AR9D2` works and the allocation of the AR9 blocks
// is checked in one place; `validate` never emits them.
// TestAllocatedBlocksCoverLiteralsInOtherPackages keeps them equal to the
// constants in internal/skillsearch.
const (
	// AnalyzerSearch is the analyzer name of the skill search codes.
	AnalyzerSearch = "search"

	CodeSearchCasesInvalid   = "AR9D2"
	CodeSearchEvalRegression = "AR9D4"
)

func init() {
	registerRules(
		RuleInfo{CodeSearchCasesInvalid, "search-cases-invalid", SeverityError, "a skill search cases file cannot be used: not valid JSON, unknown key, unsupported schema version or an invalid case (search --eval only)"},
		RuleInfo{CodeSearchEvalRegression, "search-eval-regression", SeverityError, "a skill search metric is below its minimum, or more cases regressed against the baseline than allowed (search --eval only)"},
	)
	registerRuleDocs(map[string]RuleDoc{
		CodeSearchCasesInvalid: {
			Why:  "A cases file that does not parse or validate would silently measure nothing, so `search --eval` refuses it and names every problem. Only `ai-rulez search --eval` reports it.",
			Bad:  "A case without a query, or a file with `schema_version = 2`",
			Good: "Fix the listed problems; every case needs a query and the expected skills",
		},
		CodeSearchEvalRegression: {
			Why:  "Search quality fell below the minimum the cases file sets, or more cases flipped from hit to miss against the baseline than allowed. Only `ai-rulez search --eval` reports it and exits non-zero.",
			Bad:  "A skill description rewrite that drops recall@5 under the configured minimum",
			Good: "Restore the discoverability of the skill, or lower the minimum deliberately",
		},
	})
	SetAnalyzer(CodeSearchCasesInvalid, AnalyzerSearch, ScopeItem)
	SetAnalyzer(CodeSearchEvalRegression, AnalyzerSearch, ScopeBundle)
}
