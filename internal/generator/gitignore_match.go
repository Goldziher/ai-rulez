package generator

import (
	"path"
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

// matchesPattern reports whether a path matches a gitignore pattern with git's
// own semantics: a pattern with no slash matches the last segment at any depth,
// one with a slash (leading or inside) is anchored at the root of the .gitignore
// that holds it, "**" crosses directories, and a trailing "/" covers a
// directory's contents. A pattern is never matched as a mere substring, and a
// negation does not ignore anything (the caller reads it separately) — dropping
// a managed entry because a hand-written line happened to contain it hides files
// the user expects ignored (#519).
func matchesPattern(filename, pattern string) bool {
	if pattern == "" || strings.HasPrefix(pattern, "!") || strings.HasPrefix(pattern, "#") {
		return false
	}
	candidate := strings.Trim(strings.TrimSuffix(filepathSlash(filename), "/"), "/")
	if candidate == "" {
		return false
	}
	explicitlyAnchored := strings.HasPrefix(pattern, "/")
	body := strings.Trim(strings.TrimSuffix(pattern, "/"), "/")
	if body == "" {
		return false
	}
	if !explicitlyAnchored && !strings.Contains(body, "/") {
		if globMatch(body, path.Base(candidate)) {
			return true
		}
		return coversAncestor(body, candidate)
	}
	regex := globRegexp(body)
	segments := strings.Split(candidate, "/")
	for i := range segments {
		if regex.MatchString(strings.Join(segments[:i+1], "/")) {
			return true
		}
	}
	return false
}

// filepathSlash normalizes a path to forward slashes without importing
// path/filepath into this file's matcher.
func filepathSlash(p string) string { return strings.ReplaceAll(p, "\\", "/") }
