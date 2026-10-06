package lint

// Codes of the `ai-rulez convert` report (internal/importer, docs/cli.md). They
// are registered here so `validate --explain AR9F1` works and the allocation of
// the AR9 blocks is checked in one place; `validate` never emits them.
// TestAllocatedBlocksCoverLiteralsInOtherPackages keeps them equal to the
// constants in internal/importer.
const (
	CodeConvertInvalid      = "AR9F0"
	CodeConvertApproximated = "AR9F1"
	CodeConvertDropped      = "AR9F2"
	CodeConvertNeedsAction  = "AR9F3"
	CodeConvertUnsupported  = "AR9F4"
	CodeConvertBlockedScan  = "AR9F5"
)

func init() {
	registerRules(
		RuleInfo{CodeConvertInvalid, "convert-input-invalid", SeverityError, "an input file of `convert` cannot be parsed at all (reported by convert, never by validate)"},
		RuleInfo{CodeConvertApproximated, "convert-approximated", SeverityWarning, "a construct was converted with a different meaning or without part of its fields (convert report only)"},
		RuleInfo{CodeConvertDropped, "convert-dropped", SeverityWarning, "a construct has no equivalent and was not converted (convert report only)"},
		RuleInfo{CodeConvertNeedsAction, "convert-needs-action", SeverityWarning, "a construct needs a manual step after conversion, for example a literal secret replaced by ${VAR} (convert report only)"},
		RuleInfo{CodeConvertUnsupported, "convert-unsupported", SeverityWarning, "a source or construct convert does not support (convert report only)"},
		RuleInfo{CodeConvertBlockedScan, "convert-blocked-by-scan", SeverityError, "the security scan of the planned tree blocked the write (convert report only)"},
	)
	registerRuleDocs(map[string]RuleDoc{
		CodeConvertInvalid: {
			Why:  "A tool file that cannot be parsed cannot be translated, so the run stops instead of writing a partial tree. Only `ai-rulez convert` reports it.",
			Bad:  "`.cursor/rules/a.mdc` with a broken YAML header, or a `skills-lock.json` that is not JSON",
			Good: "Fix or remove the input file and run `ai-rulez convert` again",
		},
		CodeConvertApproximated: {
			Why:  "The target has no field with the same meaning, so the value was kept in the closest form (for example an unknown skill frontmatter key that only some presets render). Only the convert report carries it.",
			Bad:  "A Cursor rule with `alwaysApply: true` and `globs`: the globs are dropped as always-on makes them moot",
			Good: "Review the converted file and adjust the frontmatter if the approximation is not what you want",
		},
		CodeConvertDropped: {
			Why:  "Nothing in ai-rulez expresses the construct, so it is not in the converted tree. Only the convert report carries it.",
			Bad:  "An unknown rule frontmatter key in a Cursor rule",
			Good: "Re-create the intent by hand if it matters, or accept the loss",
		},
		CodeConvertNeedsAction: {
			Why:  "The conversion is incomplete until a person acts: a literal credential in an MCP server was replaced by a `${VAR}` reference, a lock hash was not carried over, a hook or an allow rule was written disabled, or a remote source was not fetched. Only the convert report carries it.",
			Bad:  "An MCP server with `--api-key sk-...` in its arguments",
			Good: "Export the variable named in the reference and run `ai-rulez lock`, review a disabled hook and uncomment it (or rerun with `--enable-hooks`), or rerun with `--fetch`",
		},
		CodeConvertUnsupported: {
			Why:  "The source kind is out of scope (a `node_modules` or local skills-lock source, a transport helper URL, an SSH or marketplace APM dependency, an MCP registry reference, an npm rulesync source). Only the convert report carries it.",
			Bad:  "A skills-lock entry whose source is `file:///tmp/skill`",
			Good: "Install the skill from an https or ssh Git source and convert again",
		},
		CodeConvertBlockedScan: {
			Why:  "The planned tree failed the security scan (the AR0xx family), so nothing was written. Only `ai-rulez convert` reports it.",
			Bad:  "A converted rule that contains `curl ... | sh`",
			Good: "Remove or rewrite the flagged text in the source file and convert again",
		},
	})
}
