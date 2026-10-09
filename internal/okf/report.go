package okf

import "fmt"

// FailNone is the --fail-on value that never fails a run.
const FailNone = "none"

// ParseFailOn reads a --fail-on value: a severity, or FailNone (the second
// result), the empty string meaning error.
func ParseFailOn(value string) (threshold Severity, none bool, err error) {
	switch value {
	case "":
		return SeverityError, false, nil
	case FailNone:
		return SeverityError, true, nil
	case string(SeverityError), string(SeverityWarning), string(SeverityInfo):
		return Severity(value), false, nil
	}
	return "", false, fmt.Errorf("unknown --fail-on %q (use error, warning, info or none)", value)
}

func rank(s Severity) int {
	switch s {
	case SeverityError:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	}
	return 0
}

// Fails reports whether any finding is at or above threshold.
func Fails(findings []Finding, threshold Severity) bool {
	for i := range findings {
		if rank(findings[i].Severity) >= rank(threshold) {
			return true
		}
	}
	return false
}

// Check runs every conformance and hygiene check on a bundle: the findings
// `ai-rulez okf validate` reports.
func Check(b *Bundle) []Finding {
	return append(b.CheckRoot(), b.Validate()...)
}

// ValidationDocument is the JSON document of `okf validate --format json`; the
// okf_validate tool returns the same one. spec names the bundle as the caller
// gave it.
func ValidationDocument(spec string, b *Bundle, findings []Finding) map[string]any {
	if findings == nil {
		findings = []Finding{}
	}
	return map[string]any{
		"bundle": spec, "okf_spec": SpecVersion,
		"concepts": len(b.Concepts), "index_style": b.IndexStyle(), "findings": findings,
	}
}
