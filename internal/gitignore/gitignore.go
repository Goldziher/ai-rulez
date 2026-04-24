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

// ReplaceFencedBlock replaces the content between BEGIN and END markers
func ReplaceFencedBlock(content, newBlock string) string {
	lines := strings.Split(content, "\n")
	var result strings.Builder
	inBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == BeginMarker {
			inBlock = true
			continue
		}
		if trimmed == EndMarker {
			inBlock = false
			continue
		}
		if !inBlock {
			result.WriteString(line + "\n")
		}
	}

	// Remove trailing newlines to avoid accumulating blank lines
	out := strings.TrimRight(result.String(), "\n")
	if out != "" {
		out += "\n\n"
	}
	out += newBlock
	return out
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
