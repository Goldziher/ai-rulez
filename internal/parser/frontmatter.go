package parser

import (
	"bufio"
	"strings"

	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// MetadataV3 represents frontmatter metadata parsed from markdown files
// This is a local type to avoid circular imports with the config package
type MetadataV3 struct {
	Priority string
	Targets  []string
	Extra    map[string]string
}

// ParseFrontmatter parses optional YAML frontmatter from content
// Returns metadata (nil if none) and the actual content (without frontmatter)
// Properly handles Windows CRLF line endings by normalizing to LF first
// Returns error if frontmatter is present but malformed (non-silent failure)
func ParseFrontmatter(content string) (*MetadataV3, string, error) {
	// Normalize line endings: CRLF -> LF (fixes Windows CRLF issue #8)
	normalizedContent := normalizeLFLineEndings(content)

	// Check if content starts with ---
	if !strings.HasPrefix(normalizedContent, "---\n") {
		return nil, normalizedContent, nil
	}

	// Find the closing ---
	lines := strings.Split(normalizedContent, "\n")
	if len(lines) < 3 {
		return nil, normalizedContent, nil
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			endIdx = i
			break
		}
	}

	if endIdx == -1 {
		// No closing ---, treat as regular content
		return nil, normalizedContent, nil
	}

	// Extract frontmatter YAML
	frontmatterLines := lines[1:endIdx]
	frontmatterYAML := strings.Join(frontmatterLines, "\n")

	// Parse frontmatter - return error instead of silently ignoring (fixes issue #4)
	var metadata MetadataV3
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &metadata); err != nil {
		return nil, normalizedContent, oops.
			With("content_preview", truncateString(frontmatterYAML, 100)).
			Hint("Check the YAML syntax in your frontmatter - ensure proper indentation and formatting\nCommon issues: tabs instead of spaces, missing colons, incorrect indentation").
			Wrapf(err, "parse frontmatter YAML")
	}

	// Extract actual content (after frontmatter)
	actualContent := strings.Join(lines[endIdx+1:], "\n")
	actualContent = strings.TrimPrefix(actualContent, "\n")

	return &metadata, actualContent, nil
}

// ParseFrontmatterNonFatal parses frontmatter but returns only metadata and content
// This version logs warnings for parse errors instead of returning them
// Useful for cases where frontmatter errors should not block processing
func ParseFrontmatterNonFatal(content string) (metadata *MetadataV3, body string) {
	metadata, actualContent, err := ParseFrontmatter(content)
	if err != nil {
		// Log warning but continue (non-fatal)
		// Return the original content as-is if frontmatter parsing fails
		return nil, content
	}
	return metadata, actualContent
}

// normalizeLFLineEndings converts CRLF (Windows) line endings to LF (Unix) format
// This ensures consistent line splitting across all platforms
// Fixes issue #8: Windows CRLF line ending corruption
func normalizeLFLineEndings(content string) string {
	// Replace CRLF with LF
	content = strings.ReplaceAll(content, "\r\n", "\n")
	// Also handle old Mac line endings (CR only) just in case
	content = strings.ReplaceAll(content, "\r", "\n")
	return content
}

// scanLinesPreservingEmpty scans lines from content while preserving empty lines
// Uses bufio.Scanner internally for robust line handling
// This is an alternative approach that can be used for more robust parsing
func scanLinesPreservingEmpty(content string) []string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

// truncateString truncates a string to max length and adds ellipsis if truncated
// Used for error message previews
func truncateString(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
