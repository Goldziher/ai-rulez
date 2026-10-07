package lint

// Codes of `ai-rulez verifiers run` (internal/verifiers, docs/verifiers.md).
// They are registered here so `validate --explain AR9H1` works and the
// allocation of the AR9 blocks is checked in one place; `validate` emits them
// only with --strict --verifiers. TestAllocatedBlocksCoverLiteralsInOtherPackages
// keeps them equal to the constants in internal/verifiers.
const (
	// AnalyzerVerifiers is the analyzer name of the verifier codes.
	AnalyzerVerifiers = "verifiers"

	CodeVerifierFailed    = "AR9H1"
	CodeVerifierInvalid   = "AR9H2"
	CodeVerifierCommand   = "AR9H3"
	CodeVerifierLLM       = "AR9H4"
	CodeVerifierDeadScope = "AR9H5"
	CodeVerifierNoExample = "AR9H6"
)

func registerVerifiercodes(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeVerifierFailed, "verifier-failed", SeverityWarning, "a verifier's predicate did not hold; the finding names the verifier and the rule or skill that declared it (reported by `verifiers run` and `validate --strict --verifiers`)"},
		RuleInfo{CodeVerifierInvalid, "verifier-invalid", SeverityError, "a verifier declaration is unusable: bad regex, unknown or missing target, two predicates, bad template, unknown key (reported by `verifiers run` and `validate --strict --verifiers`)"},
		RuleInfo{CodeVerifierCommand, "verifier-command-failed-to-run", SeverityError, "a command predicate was refused (no --allow-exec, an untrusted include) or did not run: not found, could not start, timed out (reported by `verifiers run` and `validate --strict --verifiers`)"},
		RuleInfo{CodeVerifierLLM, "verifier-llm-skipped", SeverityInfo, "an llm verifier was not evaluated: LLM use is off, the budget would be exceeded, the content was withheld or unreadable, or --estimate was given; never counted as a pass (reported by `verifiers run` and `validate --strict --verifiers`)"},
		RuleInfo{CodeVerifierDeadScope, "verifier-dead-scope", SeverityWarning, "a verifier's when_changed matches no file in the repository, so it can never apply (reported by `verifiers run` and `validate --strict --verifiers`, with --strict-applicability)"},
		RuleInfo{CodeVerifierNoExample, "verifier-no-examples", SeverityWarning, "a verifier has no self-test examples (reported by `verifiers run` and `validate --strict --verifiers`, with [verifiers_settings] require_examples)"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeVerifierFailed: {
			Why:  "The check a rule or skill declared with a verifier does not hold on the evaluated files. Severity is the verifier's own (warning unless it sets `severity`). `ai-rulez verifiers run` and `validate --strict --verifiers` report it.",
			Bad:  "A migration `db/migrations/0042.sql` without a `-- down` section while verifier `migrations-have-down` requires one",
			Good: "Apply the verifier's `fix`: add the section, or change the verifier if the rule changed",
		},
		CodeVerifierInvalid: {
			Why:  "A declaration under `.ai-rulez/verifiers/` that cannot be used is reported instead of silently skipped, so a typo never disables a check. `ai-rulez verifiers run`, `list`, `test` and `validate --strict --verifiers` report it.",
			Bad:  "`rule = \"ghost\"` naming a rule that does not exist, or `regex = \"(\"`",
			Good: "Name an existing rule, skill, agent or command and a valid RE2 regex",
		},
		CodeVerifierCommand: {
			Why:  "A command predicate runs a program, which only happens with `--allow-exec` (or AI_RULEZ_VERIFIERS_ALLOW_EXEC=1) and, for a verifier that came from an include, only when `[verifiers_settings] trust_exec_from` names that include. A command that is refused, cannot be started or times out is an error, never a pass, even under `not`. `ai-rulez verifiers run`, `test` and `validate --strict --verifiers` report it.",
			Bad:  "`argv = [\"make\", \"check-lock\"]` run in CI without `--allow-exec`, or a program that is not installed",
			Good: "Pass `--allow-exec` for trusted refs only, install the program, or raise `timeout_s` (capped by `max_timeout_s`)",
		},
		CodeVerifierLLM: {
			Why:  "An `llm` verifier sends the changed hunks to a model, so it runs only with `--allow-llm`, with `allow_network = true` set in the user config, a configured model and a budget. When any of that is missing, the estimate exceeds `--max-cost`, or every hunk was withheld (a secret or hidden characters) or every changed file was binary or too large, the verifier is skipped and shown as skipped, never as passed. `ai-rulez verifiers run` and `validate --strict --verifiers` report it.",
			Bad:  "An `llm` verifier in CI without `--allow-llm`, which silently looks green",
			Good: "Pass `--allow-llm` where model use is allowed, or read the skipped line as 'not checked'",
		},
		CodeVerifierNoExample: {
			Why:  "With `[verifiers_settings] require_examples = true` a verifier without `[[verifiers.examples]]` has no self-test, so a regex typo can go unnoticed. `ai-rulez verifiers run` and `validate --strict --verifiers` report it.",
			Bad:  "A spec verifier with a `forbid` regex and no examples",
			Good: "Add a passing and a failing example and run `ai-rulez verifiers test`",
		},
		CodeVerifierDeadScope: {
			Why:  "A `when_changed` glob that matches no file of the repository means the verifier silently stopped working. `ai-rulez verifiers run --strict-applicability` (or `[verifiers_settings] warn_dead`) reports it.",
			Bad:  "`when_changed = [\"src/handlres/**\"]` after a typo or a directory rename",
			Good: "Fix the glob, or delete the verifier",
		},
	})
	SetAnalyzer(CodeVerifierFailed, AnalyzerVerifiers, ScopeItem)
	SetAnalyzer(CodeVerifierInvalid, AnalyzerVerifiers, ScopeItem)
	SetAnalyzer(CodeVerifierCommand, AnalyzerVerifiers, ScopeItem)
	SetAnalyzer(CodeVerifierLLM, AnalyzerVerifiers, ScopeItem)
	SetAnalyzer(CodeVerifierNoExample, AnalyzerVerifiers, ScopeItem)
	SetAnalyzer(CodeVerifierDeadScope, AnalyzerVerifiers, ScopeItem)
}
