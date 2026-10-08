package generator

import (
	"path/filepath"
)

func gitignorePatterns(content string) []string {
	var patterns []string
	for _, line := range splitLines(content) {
		trimmed := trimSpace(line)
		if trimmed == "" || hasPrefix(trimmed, "#") {
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
	if hasSuffix(pattern, "/") {
		return matchesDirectory(filename, pattern)
	}

	// Glob pattern
	if contains(pattern, "*") || contains(pattern, "?") {
		if matched, _ := filepath.Match(pattern, filename); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, filepath.Base(filename)); matched {
			return true
		}
		return false
	}

	// Absolute pattern
	if hasPrefix(pattern, "/") {
		return filename == trimPrefix(pattern, "/")
	}

	// Substring match
	return filename == pattern ||
		hasSuffix(filename, "/"+pattern) ||
		contains(filename, "/"+pattern+"/") ||
		contains(filename, pattern)
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
	dirPrefix := trimPrefix(trimSuffix(pattern, "/"), "/")
	candidate := trimPrefix(trimSuffix(filename, "/"), "/")

	return candidate == dirPrefix || hasPrefix(candidate, dirPrefix+"/")
}
