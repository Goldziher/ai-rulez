package markdown

import (
	"regexp"
	"strings"
)

// StripFirstHeading removes the first H1 heading from markdown content.
// This is useful when embedding content that has its own title but we're already
// showing the title elsewhere (e.g., in a parent section).
func StripFirstHeading(content string) string {
	lines := strings.Split(content, "\n")

	// Find and remove the first H1 heading
	inFirstHeading := false
	result := make([]string, 0, len(lines))

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if this is an ATX-style H1 heading (starts with single #)
		if !inFirstHeading && strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ") {
			inFirstHeading = true
			continue // Skip the H1 line
		}

		// If we just skipped the H1, also skip any immediately following blank lines
		if inFirstHeading {
			if trimmed == "" {
				// Skip blank lines immediately after the heading
				// But stop skipping once we find content
				if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
					inFirstHeading = false
				}
				continue
			}
			inFirstHeading = false
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// DemoteHeadings reduces all heading levels by the specified amount.
// For example, with levels=1: H1 becomes H2, H2 becomes H3, etc.
func DemoteHeadings(content string, levels int) string {
	if levels <= 0 {
		return content
	}

	lines := strings.Split(content, "\n")
	result := make([]string, len(lines))

	// Regex to match ATX-style headings (# Heading)
	headingRegex := regexp.MustCompile(`^(#{1,6})\s+(.*)$`)

	for i, line := range lines {
		if match := headingRegex.FindStringSubmatch(line); match != nil {
			// match[1] is the hash marks, match[2] is the heading text
			currentLevel := len(match[1])
			newLevel := currentLevel + levels
			if newLevel > 6 {
				newLevel = 6 // Max heading level in markdown
			}
			result[i] = strings.Repeat("#", newLevel) + " " + match[2]
		} else {
			result[i] = line
		}
	}

	return strings.Join(result, "\n")
}

// NormalizeBlankLines ensures there are no more than 2 consecutive blank lines.
// Markdownlint complains about multiple consecutive blank lines.
func NormalizeBlankLines(content string) string {
	// Replace 3 or more consecutive newlines with 2
	multipleNewlines := regexp.MustCompile(`\n{3,}`)
	return multipleNewlines.ReplaceAllString(content, "\n\n")
}

// ProcessEmbeddedContent processes markdown content to be embedded in a larger document.
// It strips the first H1 heading and normalizes blank lines.
func ProcessEmbeddedContent(content string) string {
	// Strip the first heading since we show it elsewhere
	content = StripFirstHeading(content)

	// Normalize blank lines to avoid multiple consecutive blanks
	content = NormalizeBlankLines(content)

	// Trim leading/trailing whitespace
	content = strings.TrimSpace(content)

	return content
}
