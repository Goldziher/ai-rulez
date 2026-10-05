// Package okf reads, validates and writes Open Knowledge Format bundles
// (https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md).
//
// It implements OKF spec v0.2 and knows nothing about ai-rulez content: the
// mapping between the two lives in internal/okfbridge. The reader is tolerant
// (SPEC section 11: unknown types, unknown keys, broken links and missing
// indexes never make a bundle unreadable) and the writer is strict (it only
// emits what the spec describes, deterministically, without timestamps).
package okf

// SpecVersion is the OKF spec version this package implements.
const SpecVersion = "0.2"

// ExtensionKey is the frontmatter key ai-rulez uses to carry its own metadata
// through a bundle. SPEC section 4.1 allows any additional key.
const ExtensionKey = "x-ai-rulez"

// Reserved file names (SPEC section 3.1).
const (
	IndexFile = "index.md"
	LogFile   = "log.md"
)

// Codes of the findings this package reports. They are part of the public
// contract: never renumber or reuse one.
const (
	CodeIndexMismatch     = "AR9B0"
	CodeTypeInvalid       = "AR9B1"
	CodeLinkBroken        = "AR9B2"
	CodeVersionInvalid    = "AR9B3"
	CodeOrphan            = "AR9B4"
	CodeExportDrift       = "AR9B5"
	CodeReservedStructure = "AR9B6"
	CodeTitleDuplicate    = "AR9B7"
	CodePathUnsafe        = "AR9B8"
	CodeLossyMapping      = "AR9B9"
)

// Severity of a finding.
type Severity string

// Severities, matching the lint package's vocabulary.
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

// Rules lists every OKF check, sorted by code.
func Rules() []Rule {
	return []Rule{
		{CodeIndexMismatch, "okf-index-mismatch", SeverityWarning, "an OKF index.md lists a file that does not exist, or omits a concept or subdirectory of its directory"},
		{CodeTypeInvalid, "okf-type-invalid", SeverityError, "an OKF concept has unparseable frontmatter or a missing or empty type"},
		{CodeLinkBroken, "okf-link-broken", SeverityWarning, "a markdown link in an OKF bundle does not resolve to a file in the bundle"},
		{CodeVersionInvalid, "okf-version-invalid", SeverityWarning, "the root index okf_version is not MAJOR.MINOR, or names a version other than the one ai-rulez implements"},
		{CodeOrphan, "okf-orphan", SeverityInfo, "an OKF concept is reachable from no index entry and no link"},
		{CodeExportDrift, "okf-export-drift", SeverityError, "the OKF bundle on disk differs from what export okf would write now"},
		{CodeReservedStructure, "okf-reserved-structure", SeverityError, "an OKF index.md has frontmatter it may not have, or a log.md heading is not an ISO date"},
		{CodeTitleDuplicate, "okf-title-duplicate", SeverityInfo, "two OKF concepts in one directory share a title"},
		{CodePathUnsafe, "okf-path-unsafe", SeverityError, "an OKF bundle contains a symlink, a path escaping the bundle, or paths differing only in case"},
		{CodeLossyMapping, "okf-lossy-mapping", SeverityInfo, "an OKF concept carries x-ai-rulez data that cannot be mapped and imports as plain context"},
	}
}

// DefaultSeverity returns the default severity of a code.
func DefaultSeverity(code string) Severity {
	for _, r := range Rules() {
		if r.Code == code {
			return r.Default
		}
	}
	return SeverityWarning
}

// RuleName returns the name of a code.
func RuleName(code string) string {
	for _, r := range Rules() {
		if r.Code == code {
			return r.Name
		}
	}
	return ""
}
