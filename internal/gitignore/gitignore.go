package gitignore

import (
	"strings"
)

// Fenced block markers for managed gitignore sections
const (
	BeginMarker = "# BEGIN ai-rulez (DO NOT EDIT - managed by ai-rulez)"
	EndMarker   = "# END ai-rulez"
	OldHeader   = "# AI Rules generated files"
)

// ReplaceFencedBlock splices newBlock in place of the existing BEGIN/END
// region, keeping the lines before and after the fence where the user put
// them. With no fence present the block is appended; an empty newBlock drops
// the region entirely.
func ReplaceFencedBlock(content, newBlock string) string {
	lines := strings.Split(content, "\n")
	begin, end := -1, len(lines)
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case BeginMarker:
			if begin == -1 {
				begin = i
			}
		case EndMarker:
			if begin != -1 && end == len(lines) {
				end = i + 1
			}
		}
	}

	prefix, suffix := content, ""
	if begin != -1 {
		prefix = strings.Join(lines[:begin], "\n")
		suffix = strings.Join(lines[end:], "\n")
	}

	var parts []string
	for _, part := range []string{prefix, newBlock, suffix} {
		if trimmed := strings.Trim(part, "\n"); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	out := strings.Join(parts, "\n\n")
	if out != "" {
		out += "\n"
	}
	return out
}

// PatternsOutsideFence returns the set of trimmed, non-comment, non-empty
// pattern lines present in `content` that live outside any BEGIN/END ai-rulez
// fence. Used to skip patterns the user has already pinned manually.
func PatternsOutsideFence(content string) map[string]bool {
	patterns := make(map[string]bool)
	inBlock := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == BeginMarker {
			inBlock = true
			continue
		}
		if trimmed == EndMarker {
			inBlock = false
			continue
		}
		if inBlock || trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		patterns[trimmed] = true
	}
	return patterns
}

// ReplaceOldHeaderBlock replaces from the old-style header to end of file
func ReplaceOldHeaderBlock(content, newBlock string) string {
	lines := strings.Split(content, "\n")
	var result strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == OldHeader {
			break
		}
		result.WriteString(line + "\n")
	}

	out := strings.TrimRight(result.String(), "\n")
	if out != "" {
		out += "\n\n"
	}
	out += newBlock
	return out
}
