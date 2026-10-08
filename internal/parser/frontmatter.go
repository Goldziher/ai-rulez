package parser

import (
	"bufio"
	"strings"

	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// Metadata represents frontmatter metadata parsed from markdown files
// This is a local type to avoid circular imports with the config package.
//
// Tools, Skills, and Keywords are list-valued fields that need typed handling
// because YAML sequences cannot round-trip through map[string]string —
// they would be stringified via fmt %v ("[a b c]") instead of preserved as lists.
type Metadata struct {
	Priority string
	Targets  []string
	Tools    []string
	Skills   []string
	Keywords []string
	Extra    map[string]string `yaml:",inline"`
}

// ParseFrontmatter parses optional YAML frontmatter from content
// Returns metadata (nil if none) and the actual content (without frontmatter)
// Properly handles Windows CRLF line endings by normalizing to LF first
// Returns error if frontmatter is present but malformed (non-silent failure)
func ParseFrontmatter(content string) (*Metadata, string, error) {
	// Normalize line endings: CRLF -> LF (fixes Windows CRLF issue #8)
	// A UTF-8 byte order mark before the opening --- is not part of the content.
	normalizedContent := normalizeLFLineEndings(strings.TrimPrefix(content, "\ufeff"))

	// Check if content starts with ---
	if !strings.HasPrefix(normalizedContent, "---\n") {
		return nil, normalizedContent, nil
	}

	// Find the closing --- without splitting the whole document into lines: the
	// body can be large and only the frontmatter is parsed. lineNo counts lines
	// from the opening marker (line 0).
	const openLen = len("---\n")
	yamlEnd, rest, found := 0, "", false
	pos := openLen
	for lineNo := 1; ; lineNo++ {
		nl := strings.IndexByte(normalizedContent[pos:], '\n')
		line := normalizedContent[pos:]
		if nl >= 0 {
			line = line[:nl]
		}
		if strings.TrimSpace(line) == "---" {
			if nl < 0 && lineNo == 1 {
				// "---\n---" holds two lines: too short to be frontmatter.
				return nil, normalizedContent, nil
			}
			yamlEnd, found = openLen, true // no lines between the markers
			if pos > openLen {
				yamlEnd = pos - 1 // drop the newline that ends the last frontmatter line
			}
			if nl >= 0 {
				rest = normalizedContent[pos+nl+1:]
			}
			break
		}
		if nl < 0 {
			break
		}
		pos += nl + 1
	}

	if !found {
		// No closing ---, treat as regular content
		return nil, normalizedContent, nil
	}

	// Extract frontmatter YAML
	frontmatterYAML := normalizedContent[openLen:yamlEnd]

	// Parse frontmatter - return error instead of silently ignoring (fixes issue #4)
	var metadata Metadata
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &metadata); err != nil {
		return nil, normalizedContent, oops.
			With("content_preview", truncateString(frontmatterYAML, 100)).
			Hint("Check the YAML syntax in your frontmatter - ensure proper indentation and formatting\nCommon issues: tabs instead of spaces, missing colons, incorrect indentation").
			Wrapf(err, "parse frontmatter YAML")
	}

	// Extract actual content (after frontmatter)
	actualContent := strings.TrimPrefix(rest, "\n")

	return &metadata, actualContent, nil
}

// ParseFrontmatterNonFatal parses frontmatter but returns only metadata and content
// This version logs warnings for parse errors instead of returning them
// Useful for cases where frontmatter errors should not block processing
func ParseFrontmatterNonFatal(content string) (metadata *Metadata, body string) {
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
