package lint

// Codes of `ai-rulez search --eval` (internal/skillsearch). They are registered
// here so `validate --explain AR9D2` works and the allocation of the AR9 blocks
// is checked in one place. `validate` emits AR9D0 (the [search] table) and AR9D1
// (a committed index that is out of date); the others come from `search`.
// TestAllocatedBlocksCoverLiteralsInOtherPackages keeps them equal to the
// constants in internal/skillsearch.
const (
	// AnalyzerSearch is the analyzer name of the skill search codes.
	AnalyzerSearch = "search"

	CodeSearchConfigInvalid  = "AR9D0"
	CodeSearchIndexStale     = "AR9D1"
	CodeSearchCasesInvalid   = "AR9D2"
	CodeSearchTextWithheld   = "AR9D3"
	CodeSearchEvalRegression = "AR9D4"
)

func init() {
	registerRules(
		RuleInfo{CodeSearchConfigInvalid, "search-config-invalid", SeverityError, "the [search] table is invalid: an unknown mode, fusion or dtype, an unknown field, an out-of-range number or an index_dir that leaves the config directory"},
		RuleInfo{CodeSearchIndexStale, "search-index-stale", SeverityWarning, "a committed skill search index no longer matches the skills or the embedding model: a changed description, an added or removed skill, another model or text template"},
		RuleInfo{CodeSearchTextWithheld, "search-text-withheld", SeverityWarning, "a skill was not embedded because its text looks like it holds a secret; it ranks lexically only (search index only)"},
		RuleInfo{CodeSearchCasesInvalid, "search-cases-invalid", SeverityError, "a skill search cases file cannot be used: not valid JSON, unknown key, unsupported schema version or an invalid case (search --eval only)"},
		RuleInfo{CodeSearchEvalRegression, "search-eval-regression", SeverityError, "a skill search metric is below its minimum, or more cases regressed against the baseline than allowed (search --eval only)"},
	)
	registerRuleDocs(map[string]RuleDoc{
		CodeSearchConfigInvalid: {
			Why:  "A bad [search] table would silently change how find_skill ranks, so it is reported with the key at fault. An index_dir outside the config directory is refused so a committed config cannot make the tool read or write elsewhere.",
			Bad:  "`mode = \"semantic\"` or `index_dir = \"../shared\"`",
			Good: "`mode = \"hybrid\"` and an index_dir under the config directory",
		},
		CodeSearchIndexStale: {
			Why:  "A committed index is only useful while its vectors describe the skills as they are: after a description edit, an added or removed skill or a model change, hybrid ranking quietly falls back to lexical for the affected skills. Run `ai-rulez search index` and commit the result. Only an index_dir outside local/ is checked; the machine-local index is never a finding.",
			Bad:  "`index_dir = \"search-index\"` committed, then a skill's description edited without rebuilding",
			Good: "Re-run `ai-rulez search index` and commit the changed manifest.json and vectors.bin",
		},
		CodeSearchTextWithheld: {
			Why:  "The text sent to an embedding endpoint is name, description, triggers and keywords (and the body start with index_body). If it looks like it holds a credential, `search index` skips that skill instead of sending a masked string; it still ranks lexically. Only `ai-rulez search index` reports it.",
			Bad:  "A skill description that contains an API key",
			Good: "Remove the secret from the description; `search index` then embeds the skill",
		},
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
	SetAnalyzer(CodeSearchConfigInvalid, AnalyzerSearch, ScopeBundle)
	SetAnalyzer(CodeSearchIndexStale, AnalyzerSearch, ScopeBundle)
	SetAnalyzer(CodeSearchTextWithheld, AnalyzerSearch, ScopeItem)
	SetAnalyzer(CodeSearchCasesInvalid, AnalyzerSearch, ScopeItem)
	SetAnalyzer(CodeSearchEvalRegression, AnalyzerSearch, ScopeBundle)
}
