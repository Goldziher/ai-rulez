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
// them. Any further managed region is dropped, so a file that picked up a
// duplicate fence collapses back to one. With no fence present the block is
// appended; an empty newBlock drops the region entirely.
func ReplaceFencedBlock(content, newBlock string) string {
	var before, after []string
	var inBlock, seenFence bool
	for _, line := range strings.Split(content, "\n") {
		switch strings.TrimSpace(line) {
		case BeginMarker:
			inBlock, seenFence = true, true
			continue
		case EndMarker:
			inBlock = false
			continue
		}
		switch {
		case inBlock:
		case seenFence:
			after = append(after, line)
		default:
			before = append(before, line)
		}
	}

	out := strings.Trim(strings.Join(before, "\n"), "\n")
	if block := strings.Trim(newBlock, "\n"); block != "" {
		if out != "" {
			out += "\n\n"
		}
		out += block
	}
	if out != "" {
		out += "\n"
	}
	// Keep the tail byte for byte, dropping only the blank lines that sat
	// directly under the fence.
	if tail := strings.TrimLeft(strings.Join(after, "\n"), "\n"); tail != "" {
		if out != "" {
			out += "\n"
		}
		out += tail
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
