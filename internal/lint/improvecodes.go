package lint

// Codes of the `ai-rulez improve` report (internal/improve, docs/improve.md).
// They are registered here so `validate --explain AR9J3` works and the
// allocation of the AR9 blocks is checked in one place; `validate` emits only
// AR9J6 (a repository [improve] table `improve run` will not honor). TestAllocatedBlocksCoverLiteralsInOtherPackages keeps them equal to the
// constants in internal/improve.
const (
	CodeImproveRunStale             = "AR9J1"
	CodeImproveNoHoldout            = "AR9J2"
	CodeImprovePolicyViolation      = "AR9J3"
	CodeImproveSiblingRegression    = "AR9J4"
	CodeImproveUnderpowered         = "AR9J5"
	CodeImproveRepoOptimizerIgnored = "AR9J6"
	CodeImproveIsolationUnavailable = "AR9J7"
	CodeImprovePRRefused            = "AR9J8"
	CodeImproveAdapterRefused       = "AR9J9"
)

func registerImprovecodes(s *ruleSet) {
	s.addRules(
		RuleInfo{CodeImproveRunStale, "improve-run-stale", SeverityInfo, "a saved improve run's original digest no longer matches the skill (improve apply only)"},
		RuleInfo{CodeImproveNoHoldout, "improve-no-holdout", SeverityOff, "a skill has fewer held-out eval cases than improve needs (improve only)"},
		RuleInfo{CodeImprovePolicyViolation, "improve-policy-violation", SeverityError, "a candidate round broke the diff policy (improve report only)"},
		RuleInfo{CodeImproveSiblingRegression, "improve-sibling-regression", SeverityError, "a candidate lowered another skill's trigger recall (improve report only)"},
		RuleInfo{CodeImproveUnderpowered, "improve-underpowered", SeverityInfo, "the held-out gain of a candidate cannot be told from zero (improve report only)"},
		RuleInfo{CodeImproveRepoOptimizerIgnored, "improve-repo-optimizer-ignored", SeverityWarning, "an [improve] optimizer, env_pass or looser-than-default gate key of the repository config is not used without --trust-repo-optimizer"},
		RuleInfo{CodeImproveIsolationUnavailable, "improve-isolation-unavailable", SeverityWarning, "the requested optimizer isolation could not be applied (improve only)"},
		RuleInfo{CodeImprovePRRefused, "improve-pr-refused", SeverityError, "improve pr refused to open a pull request (improve only)"},
		RuleInfo{CodeImproveAdapterRefused, "improve-adapter-refused", SeverityError, "a bundled optimizer adapter could not run (improve only)"},
	)
	s.addDocs(map[string]RuleDoc{
		CodeImproveRunStale: {
			Why:  "The skill was edited after the run measured it, so applying the candidate would overwrite those edits or mix two baselines. `improve apply` and `improve pr` report it.",
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
		CodeImproveSiblingRegression: {
			Why:  "A rewritten description can pull prompts away from another skill. The sibling guard re-runs the trigger cases of every other skill with the offline ranker, with the original and with the candidate, and rejects a candidate that lowers any sibling's trigger recall. The round is rejected before any held-out spend. Only the improve report carries it.",
			Bad:  "A candidate description for `deploy` that now also matches the prompts of `rollback`",
			Good: "Keep the description specific to what the skill does, or fix the sibling's own triggers first",
		},
		CodeImproveUnderpowered: {
			Why:  "With few held-out cases, or a bootstrap interval of the gain that includes zero, an accepted gain is weak evidence. The report says so; `--require-ci-above-zero` turns it into a gate. Only the improve report carries it.",
			Bad:  "Accepting a +5 point gain measured on six held-out cases",
			Good: "Add held-out cases, or review the diff with the interval in mind",
		},
		CodeImproveRepoOptimizerIgnored: {
			Why:  "A repository config must not choose a command that runs on your machine or the environment variables it receives, so `[improve] optimizer` and `env_pass` in a repository config are used only with `--trust-repo-optimizer`. A repository may also only tighten the acceptance gate: `min_gain`, `max_regressions`, `holdout_fraction` and `max_skill_growth` looser than the defaults are ignored the same way. `validate --strict` and `improve run` report them.",
			Bad:  "A cloned repository whose `.ai-rulez/config.toml` sets `[improve] optimizer`, run with plain `improve run <skill>`",
			Good: "Pass `--with`, or review the config and add `--trust-repo-optimizer`",
		},
		CodeImproveIsolationUnavailable: {
			Why:  "`--isolation require` needs a sandbox backend (sandbox-exec on macOS, bubblewrap or unshare on Linux). When none can confine the optimizer, `require` refuses to run and `auto` runs it unconfined and says so.",
			Bad:  "`improve run --isolation require` on a system with no usable sandbox backend",
			Good: "Install bubblewrap, run in a container, or use `--isolation auto` knowing the optimizer is not confined",
		},
		CodeImprovePRRefused: {
			Why:  "`improve pr` refuses when it cannot make the pull request safely: the run is not accepted or not signed by this user, the skill at the base differs from the run's original, the branch exists, or git or the base ref is unusable.",
			Bad:  "`ai-rulez improve pr imp-1a2b3c4d --base release` when the skill differs on `release`",
			Good: "Re-run `improve run` against the base, or pick the base the run measured",
		},
		CodeImproveAdapterRefused: {
			Why:  "A bundled adapter (`builtin:review-fix`) needs a model, the network opt-in and a declared egress host; it never calls a model on its own. It refuses when one is missing.",
			Bad:  "`improve run --with builtin:review-fix` without `[llm] model` and `allow_network = true` in the user config",
			Good: "Set the model and the opt-in in the user config, and pass `--egress` for the provider host",
		},
	})
}
