package agentplugins

// Rule codes under which ai-rulez reports findings (validate --strict, publish).
// They are registered, with explanations, in internal/lint and documented in
// docs/strict-validation.md; the block is AR9O.
const (
	RuleManifest    = "AR9O0"
	RuleSkill       = "AR9O1"
	RuleMCP         = "AR9O2"
	RulePlaceholder = "AR9O3"
	RuleDropped     = "AR9O4"
	RuleUnsafe      = "AR9O5"
)

// RuleCode maps a finding code onto the AR9O rule that reports it.
func RuleCode(findingCode string) string {
	switch findingCode {
	case CodeSkillsLocation, CodeSkillMissing, CodeSkillInvalid, CodeSkillUnknownField:
		return RuleSkill
	case CodeMCPInvalid, CodeMCPSpecMismatch, CodeServerInvalid, CodeCommandNotBundled, CodeCredentialHeader:
		return RuleMCP
	case CodePlaceholder, CodePlaceholderRewritten:
		return RulePlaceholder
	case CodeServerDisabled, CodeFieldIgnored:
		return RuleDropped
	case CodePathEscape, CodeUnreadable:
		return RuleUnsafe
	default:
		return RuleManifest
	}
}
