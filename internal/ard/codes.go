package ard

// Rule codes under which ai-rulez reports findings (validate --strict, publish).
// They are registered, with explanations, in internal/lint and documented in
// docs/strict-validation.md; the block is AR9S.
const (
	CodeSchema      = "AR9S0"
	CodeIdentifier  = "AR9S1"
	CodeEntry       = "AR9S2"
	CodeQueries     = "AR9S3"
	CodeNotDeclared = "AR9S4"
)

// Code maps a finding rule onto the AR9S code that reports it.
func Code(rule string) string {
	switch rule {
	case RuleIdentifier:
		return CodeIdentifier
	case RuleEntry:
		return CodeEntry
	case RuleQueries:
		return CodeQueries
	default:
		return CodeSchema
	}
}
