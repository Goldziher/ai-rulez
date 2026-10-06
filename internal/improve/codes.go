// Package improve runs an external skill optimizer against a throwaway copy of
// an authored skill and accepts its candidate only when a held-out eval set
// improves without regressions and the diff policy and security scan hold.
// It never writes to the authored sources except through Apply, and never
// commits. The command is experimental.
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
)
