package generator

import (
	"path/filepath"
	"strings"
)

func gitignorePatterns(content string) []string {
	var patterns []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		patterns = append(patterns, trimmed)
	}
	return patterns
}

// isIgnored checks if a filename matches any gitignore pattern
func isIgnored(filename string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchesPattern(filename, pattern) {
			return true
		}
	}
	return false
}

// matchesPattern checks if a filename matches a gitignore pattern
func matchesPattern(filename, pattern string) bool {
	// Exact match
	if pattern == filename {
		return true
	}

	// Directory pattern
	if strings.HasSuffix(pattern, "/") {
		return matchesDirectory(filename, pattern)
	}

	// Glob pattern
	if strings.Contains(pattern, "*") || strings.Contains(pattern, "?") {
		if matched, _ := filepath.Match(pattern, filename); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, filepath.Base(filename)); matched {
			return true
		}
		return false
	}

	// Absolute pattern
	if strings.HasPrefix(pattern, "/") {
		return filename == strings.TrimPrefix(pattern, "/")
	}

	// Substring match
	return filename == pattern ||
		strings.HasSuffix(filename, "/"+pattern) ||
		strings.Contains(filename, "/"+pattern+"/") ||
		strings.Contains(filename, pattern)
}

// matchesDirectory checks if filename matches a directory pattern.
//
// A directory pattern covers the directory itself and everything nested under
// it, so the nesting check has to run whether or not filename is itself a
// directory: a pre-existing ".claude/" already ignores ".claude/skills/", and
// re-listing the subdirectory inside the managed fence would duplicate it.
// Trailing slashes and the leading anchor are notation, not path segments, so
// both sides are normalised before comparing.
func matchesDirectory(filename, pattern string) bool {
	dirPrefix := strings.TrimPrefix(strings.TrimSuffix(pattern, "/"), "/")
	candidate := strings.TrimPrefix(strings.TrimSuffix(filename, "/"), "/")

	return candidate == dirPrefix || strings.HasPrefix(candidate, dirPrefix+"/")
}
