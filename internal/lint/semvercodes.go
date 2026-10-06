package lint

// Codes of version-constraint resolution (internal/tagresolve, docs/lockfile.md
// "Version constraints"). `lock`, `lock --outdated` and `update` raise them, and
// a config that sets both `ref` and `version` fails validation with AR731; they
// are registered here so `validate --explain AR730` works and the AR73x block is
// checked with the others. TestAllocatedBlocksCoverLiteralsInOtherPackages keeps
// them equal to the constants in internal/tagresolve.
const (
	CodeConstraintUnsatisfiable = "AR730"
	CodeConstraintInvalid       = "AR731"
	CodeTagMoved                = "AR732"
	CodeLockedTagMissing        = "AR735"
)

func init() {
	registerRules(
		RuleInfo{CodeConstraintUnsatisfiable, "constraint-unsatisfiable", SeverityError, "no tag of the source satisfies its version constraint (or the source has no semantic version tags)"},
		RuleInfo{CodeConstraintInvalid, "constraint-invalid", SeverityError, "a version constraint does not parse, or an include, installed skill or skill source sets both ref and version"},
		RuleInfo{CodeTagMoved, "tag-moved", SeverityError, "a tag pinned in ai-rulez.lock now points to another commit; it is never followed silently"},
		RuleInfo{CodeLockedTagMissing, "locked-tag-missing", SeverityWarning, "a tag pinned in ai-rulez.lock no longer exists on the remote; the pinned commit is still used"},
	)
	registerRuleDocs(map[string]RuleDoc{
		CodeConstraintUnsatisfiable: {
			Why:  "A range such as `^1.2` is resolved against the repository's semantic-version tags. With no matching tag there is nothing to pin, and falling back to a branch would change what the constraint means.",
			Bad:  "`version = \"^3\"` on a repository whose highest tag is `v2.4.0`, or on a repository with no version tags",
			Good: "Widen the constraint, set `tag_prefix` for a monorepo, set `include_prerelease = true`, or pin a commit SHA with `ref`",
		},
		CodeConstraintInvalid: {
			Why:  "`ref` names a git ref and `version` a range; setting both leaves it unclear which one the lock records. A constraint that does not parse cannot be resolved.",
			Bad:  "`ref = \"main\"` together with `version = \"^1\"`, or `version = \"^^1\"`",
			Good: "Use one of them: `version = \"^1.2\"` (or `ref = \"^1.2\"` as shorthand), npm-style syntax",
		},
		CodeTagMoved: {
			Why:  "A tag is a promise that a version never changes. A tag that moved after it was pinned is a force-push, the way a compromised maintainer or a rewritten release shows up.",
			Bad:  "`v1.2.4` was pinned at commit 0f3e and the remote now has it at b21c",
			Good: "Review the new commit, then run `ai-rulez update <source> --accept-moved-tag` to re-pin it",
		},
		CodeLockedTagMissing: {
			Why:  "The tag was deleted from the remote. The pinned commit may still be fetchable, so generation continues, but the version label can no longer be checked.",
			Bad:  "A release tag removed after the lock was written",
			Good: "Run `ai-rulez update` to move to an existing tag, or pin the commit SHA",
		},
	})
	SetAnalyzer(CodeConstraintUnsatisfiable, AnalyzerLock, ScopeBundle)
	SetAnalyzer(CodeConstraintInvalid, AnalyzerConfig, ScopeBundle)
	SetAnalyzer(CodeTagMoved, AnalyzerLock, ScopeBundle)
	SetAnalyzer(CodeLockedTagMissing, AnalyzerLock, ScopeBundle)
}
