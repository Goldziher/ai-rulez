// Package llmstxt renders and validates files in the llms.txt format
// (https://llmstxt.org/).
//
// An llms.txt file is markdown with a fixed shape: an H1 title (the only
// required element), an optional blockquote summary, optional detail blocks
// without headings, and zero or more H2 file-list sections, each a markdown
// list whose entries hold a [name](url) link with optional notes after a colon.
// An H2 section named "Optional" holds secondary links an agent may skip.
//
// The renderer is deterministic: its output depends only on its input. The
// validator accepts any file, not only the ones this package renders.
package llmstxt

// Codes of the findings the validator reports. They are part of the public
// contract: never renumber or reuse one.
const (
	CodeTitleMissing            = "AR9P0"
	CodeSummaryMisplaced        = "AR9P1"
	CodeHeadingInvalid          = "AR9P2"
	CodeLinkEntryInvalid        = "AR9P3"
	CodeOptionalMisplaced       = "AR9P4"
	CodeSectionDuplicateOrEmpty = "AR9P5"
	CodeLinkTargetInvalid       = "AR9P6"
)

// Severity of a finding, matching the lint package's vocabulary.
type Severity string

// Severities.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Rule describes one check.
type Rule struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Default  Severity `json:"default_severity"`
	Describe string   `json:"description"`
}

// Rules lists every llms.txt check, sorted by code.
func Rules() []Rule {
	return []Rule{
		{CodeTitleMissing, "llmstxt-title-missing", SeverityError, "an llms.txt file does not start with a single H1 title"},
		{CodeSummaryMisplaced, "llmstxt-summary-misplaced", SeverityWarning, "the llms.txt summary blockquote is empty or does not directly follow the H1 title"},
		{CodeHeadingInvalid, "llmstxt-heading-invalid", SeverityError, "an llms.txt file has a heading other than the H1 title and H2 section names, or an H2 without a name"},
		{CodeLinkEntryInvalid, "llmstxt-link-entry-invalid", SeverityError, "an llms.txt file-list section has content that is not a list entry holding a [name](url) link"},
		{CodeOptionalMisplaced, "llmstxt-optional-misplaced", SeverityWarning, "the llms.txt Optional section is not the last section"},
		{CodeSectionDuplicateOrEmpty, "llmstxt-section-duplicate-or-empty", SeverityWarning, "an llms.txt section has no entries, or repeats the name of an earlier section"},
		{CodeLinkTargetInvalid, "llmstxt-link-target-invalid", SeverityWarning, "an llms.txt link has an empty target"},
	}
}

func ruleName(code string) string {
	for _, r := range Rules() {
		if r.Code == code {
			return r.Name
		}
	}
	return ""
}

func ruleSeverity(code string) Severity {
	for _, r := range Rules() {
		if r.Code == code {
			return r.Default
		}
	}
	return SeverityWarning
}
