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
	CodeReleaseHeldBack         = "AR733"
	CodeSourceOutdated          = "AR734"
	CodeLockedTagMissing        = "AR735"
)

func registerSemvercodes(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeConstraintUnsatisfiable, "constraint-unsatisfiable", SeverityError, "no tag of the source satisfies its version constraint (or the source has no semantic version tags)"},
		RuleInfo{CodeConstraintInvalid, "constraint-invalid", SeverityError, "a version constraint does not parse, or an include, installed skill or skill source sets both ref and version"},
		RuleInfo{CodeTagMoved, "tag-moved", SeverityError, "a tag pinned in ai-rulez.lock now points to another commit; it is never followed silently"},
		RuleInfo{CodeReleaseHeldBack, "release-held-back", SeverityInfo, "a tag that satisfies a version constraint was held back by min_release_age; the next older tag (or the pin) is used"},
		RuleInfo{CodeSourceOutdated, "source-outdated", SeverityOff, "a remote source has a newer tag its version constraint allows; reported by `lock --outdated` once enabled in [lint.severity]"},
		RuleInfo{CodeLockedTagMissing, "locked-tag-missing", SeverityWarning, "a tag pinned in ai-rulez.lock no longer exists on the remote; the pinned commit is still used"},
	)
	s.addDocs(map[string]RuleDoc{
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
		CodeReleaseHeldBack: {
			Why:  "A tag published a moment ago may be a compromised release. `min_release_age` waits until the tag has existed for that long, and `lock` and `update` take the newest tag that is old enough. The release time comes from the forge, else from the first time this machine saw the tag, else from the commit date (`[lock] min_release_age_source`).",
			Bad:  "`min_release_age = \"7d\"` and `v1.3.1` was released two days ago: `lock` pins `v1.3.0` instead",
			Good: "Wait for the tag to age, lower `min_release_age`, or run `ai-rulez lock --outdated` on a schedule so the first-seen record starts early",
		},
		CodeSourceOutdated: {
			Why:  "A pin that never moves falls behind security fixes. This finding is off by default; turn it on to make a scheduled `ai-rulez lock --outdated` fail (error) or warn (warning) when a source has a newer tag its constraint allows.",
			Bad:  "`v1.2.4` is pinned and `v1.3.1` satisfies `^1.2`",
			Good: "Run `ai-rulez update` after reviewing the diff, or leave the rule off",
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
	SetAnalyzer(CodeReleaseHeldBack, AnalyzerLock, ScopeBundle)
	SetAnalyzer(CodeSourceOutdated, AnalyzerLock, ScopeBundle)
	SetAnalyzer(CodeLockedTagMissing, AnalyzerLock, ScopeBundle)
}
