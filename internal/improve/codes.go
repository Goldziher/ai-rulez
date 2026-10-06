// Package improve runs an external skill optimizer against a throwaway copy of
// an authored skill and accepts its candidate only when a held-out eval set
// improves without regressions and the diff policy and security scan hold.
// It never writes to the authored sources except through Apply and the
// worktree of PR, and never commits to the user's checkout. The command is
// experimental.
package improve

// Report codes of `ai-rulez improve` (docs/improve.md). They are registered in
// internal/lint (improvecodes.go) so `validate --explain AR9J1` works and the
// allocation of the AR9 blocks is checked in one place; `validate` never emits
// them.
const (
	// CodeRunStale: the skill changed since the run, so its candidate no longer applies.
	CodeRunStale = "AR9J1"
	// CodeNoHoldout: the skill has fewer held-out cases than `improve` needs.
	CodeNoHoldout = "AR9J2"
	// CodePolicyViolation: a candidate round broke the diff policy.
	CodePolicyViolation = "AR9J3"
	// CodeSiblingRegression: a candidate lowered a sibling skill's trigger recall.
	CodeSiblingRegression = "AR9J4"
	// CodeUnderpowered: the held-out gain cannot be told from zero.
	CodeUnderpowered = "AR9J5"
	// CodeRepoOptimizerIgnored: a repository [improve] optimizer was not used.
	CodeRepoOptimizerIgnored = "AR9J6"
	// CodeIsolationUnavailable: the requested optimizer isolation could not be applied.
	CodeIsolationUnavailable = "AR9J7"
	// CodePRRefused: `improve pr` refused to open a pull request.
	CodePRRefused = "AR9J8"
	// CodeAdapterRefused: a bundled optimizer adapter could not run.
	CodeAdapterRefused = "AR9J9"
)
