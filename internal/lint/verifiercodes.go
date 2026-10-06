package lint

// Codes of `ai-rulez verifiers run` (internal/verifiers, docs/verifiers.md).
// They are registered here so `validate --explain AR9H1` works and the
// allocation of the AR9 blocks is checked in one place; `validate` never emits
// them. TestAllocatedBlocksCoverLiteralsInOtherPackages keeps them equal to
// the constants in internal/verifiers.
const (
	// AnalyzerVerifiers is the analyzer name of the verifier codes.
	AnalyzerVerifiers = "verifiers"

	CodeVerifierFailed    = "AR9H1"
	CodeVerifierInvalid   = "AR9H2"
	CodeVerifierDeadScope = "AR9H5"
)

func init() {
	registerRules(
		RuleInfo{CodeVerifierFailed, "verifier-failed", SeverityWarning, "a verifier's predicate did not hold; the finding names the verifier and the rule or skill that declared it (verifiers report only)"},
		RuleInfo{CodeVerifierInvalid, "verifier-invalid", SeverityError, "a verifier declaration is unusable: bad regex, unknown or missing target, two predicates, bad template, unknown key (verifiers report only)"},
		RuleInfo{CodeVerifierDeadScope, "verifier-dead-scope", SeverityWarning, "a verifier's when_changed matches no file in the repository, so it can never apply (verifiers report only, with --strict-applicability)"},
	)
	registerRuleDocs(map[string]RuleDoc{
		CodeVerifierFailed: {
			Why:  "The check a rule or skill declared with a verifier does not hold on the evaluated files. Severity is the verifier's own (warning unless it sets `severity`). Only `ai-rulez verifiers run` reports it.",
			Bad:  "A migration `db/migrations/0042.sql` without a `-- down` section while verifier `migrations-have-down` requires one",
			Good: "Apply the verifier's `fix`: add the section, or change the verifier if the rule changed",
		},
		CodeVerifierInvalid: {
			Why:  "A declaration under `.ai-rulez/verifiers/` that cannot be used is reported instead of silently skipped, so a typo never disables a check. Only `ai-rulez verifiers run`, `list` and `test` report it.",
			Bad:  "`rule = \"ghost\"` naming a rule that does not exist, or `regex = \"(\"`",
			Good: "Name an existing rule, skill, agent or command and a valid RE2 regex",
		},
		CodeVerifierDeadScope: {
			Why:  "A `when_changed` glob that matches no file of the repository means the verifier silently stopped working. Only `ai-rulez verifiers run --strict-applicability` reports it.",
			Bad:  "`when_changed = [\"src/handlres/**\"]` after a typo or a directory rename",
			Good: "Fix the glob, or delete the verifier",
		},
	})
	SetAnalyzer(CodeVerifierFailed, AnalyzerVerifiers, ScopeItem)
	SetAnalyzer(CodeVerifierInvalid, AnalyzerVerifiers, ScopeItem)
	SetAnalyzer(CodeVerifierDeadScope, AnalyzerVerifiers, ScopeItem)
}
