package lint

// Codes of the `ai-rulez improve` report (internal/improve, docs/improve.md).
// They are registered here so `validate --explain AR9J3` works and the
// allocation of the AR9 blocks is checked in one place; `validate` never emits
// them. TestAllocatedBlocksCoverLiteralsInOtherPackages keeps them equal to the
// constants in internal/improve.
const (
	CodeImproveRunStale        = "AR9J1"
	CodeImproveNoHoldout       = "AR9J2"
	CodeImprovePolicyViolation = "AR9J3"
)

func init() {
	registerRules(
		RuleInfo{CodeImproveRunStale, "improve-run-stale", SeverityInfo, "a saved improve run's original digest no longer matches the skill (improve apply only)"},
		RuleInfo{CodeImproveNoHoldout, "improve-no-holdout", SeverityOff, "a skill has fewer held-out eval cases than improve needs (improve only)"},
		RuleInfo{CodeImprovePolicyViolation, "improve-policy-violation", SeverityError, "a candidate round broke the diff policy (improve report only)"},
	)
	registerRuleDocs(map[string]RuleDoc{
		CodeImproveRunStale: {
			Why:  "The skill was edited after the run measured it, so applying the candidate would overwrite those edits or mix two baselines. Only `improve apply` reports it.",
			Bad:  "Edit `SKILL.md`, then run `ai-rulez improve apply imp-1a2b3c4d` for a run made before the edit",
			Good: "Re-run `ai-rulez improve run <skill>` against the current skill",
		},
		CodeImproveNoHoldout: {
			Why:  "The acceptance gate needs at least three scored held-out cases, one of them a negative, or a candidate cannot be judged on prompts the optimizer never saw. Only `improve` reports it.",
			Bad:  "A skill whose eval cases are all tagged for training, or only two cases in total",
			Good: "Tag at least three cases `holdout` (one with `expect_trigger: false`), or add cases so the fallback split reaches three",
		},
		CodeImprovePolicyViolation: {
			Why:  "The optimizer changed something it may not: a file outside the editable set, an executable bit, a frontmatter key such as `allowed-tools`, a script reference, a symlink, a larger token budget, or text that adds a security finding. The round is rejected before any eval spend. Only the improve report carries it.",
			Bad:  "A candidate that adds `Bash` to `allowed-tools` or a new `scripts/run.sh`",
			Good: "Keep edits to `SKILL.md` and `references/**`; widen the policy deliberately with `--allow-frontmatter` or `--allow-scripts`",
		},
	})
}
