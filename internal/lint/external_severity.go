package lint

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// Severity bands of a scanner finding, lowest first. The ai-rulez severity is
// derived from the band: critical and high are errors, medium a warning, low
// and info are info.
const (
	bandInfo = iota
	bandLow
	bandMedium
	bandHigh
	bandCritical
)

// bandNames are the spellings severity_map and max_severity accept. The ai-rulez
// names stand for the band they are rendered from.
var bandNames = map[string]int{
	"info": bandInfo, "none": bandInfo,
	"low": bandLow, "note": bandLow,
	"medium": bandMedium, "warning": bandMedium,
	"high": bandHigh, "error": bandHigh,
	"critical": bandCritical,
}

// parseBand reads a severity word; ok is false for an unknown one.
func parseBand(s string) (int, bool) {
	b, ok := bandNames[strings.ToLower(strings.TrimSpace(s))]
	return b, ok
}

// scannerBand maps one finding to a band, first match of: the severity_map
// entry for its rule id; properties.security-severity (GitHub convention: 9.0
// critical, 7.0 high, 4.0 medium, above 0 low); the result level (a JSON list
// entry's severity); the rule's default level; medium. A level word outside the
// known set counts as medium, as it did before severity mapping existed.
func scannerBand(sevMap map[string]string, f externalFinding) int {
	if b, ok := severityMapBand(sevMap, f.Rule); ok {
		return b
	}
	if f.HasScore {
		switch {
		case f.Score >= 9:
			return bandCritical
		case f.Score >= 7:
			return bandHigh
		case f.Score >= 4:
			return bandMedium
		case f.Score > 0:
			return bandLow
		}
	}
	if b, ok := levelBand(f.Severity); ok {
		return b
	}
	if b, ok := levelBand(f.DefaultLevel); ok {
		return b
	}
	return bandMedium
}

// levelBand maps a SARIF level or a JSON severity word.
func levelBand(level string) (int, bool) {
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" {
		return 0, false
	}
	if b, ok := bandNames[level]; ok {
		return b, true
	}
	return bandMedium, true
}

// severityMapBand finds the entry for rule: an exact id wins, then the longest
// matching glob, then the first by pattern order, so the result never depends on
// map iteration order.
func severityMapBand(m map[string]string, rule string) (int, bool) {
	if len(m) == 0 || rule == "" {
		return 0, false
	}
	if v, ok := m[rule]; ok {
		return parseBand(v)
	}
	patterns := make([]string, 0, len(m))
	for p := range m {
		patterns = append(patterns, p)
	}
	sort.Slice(patterns, func(i, j int) bool {
		if len(patterns[i]) != len(patterns[j]) {
			return len(patterns[i]) > len(patterns[j])
		}
		return patterns[i] < patterns[j]
	})
	for _, p := range patterns {
		if ok, err := path.Match(p, rule); err == nil && ok {
			return parseBand(m[p])
		}
	}
	return 0, false
}

// bandSeverity renders a band as an ai-rulez severity.
func bandSeverity(b int) Severity {
	switch {
	case b >= bandHigh:
		return SeverityError
	case b == bandMedium:
		return SeverityWarning
	}
	return SeverityInfo
}

// severityMapProblems lists invalid severity_map and max_severity values.
func severityMapProblems(sevMap map[string]string, maxSeverity string) []string {
	var problems []string
	patterns := make([]string, 0, len(sevMap))
	for p := range sevMap {
		patterns = append(patterns, p)
	}
	sort.Strings(patterns)
	for _, p := range patterns {
		if _, err := path.Match(p, ""); err != nil {
			problems = append(problems, fmt.Sprintf("severity_map pattern %q is not a valid glob", p))
		}
		if _, ok := parseBand(sevMap[p]); !ok {
			problems = append(problems, fmt.Sprintf("severity_map %q = %q is not critical, high, medium, low, info, error or warning", p, sevMap[p]))
		}
	}
	if maxSeverity != "" {
		if _, ok := parseBand(maxSeverity); !ok {
			problems = append(problems, fmt.Sprintf("max_severity %q is not critical, high, medium, low, info, error or warning", maxSeverity))
		}
	}
	return problems
}
