package utils

import (
	"regexp"
	"strings"
)

var (
	// sanitizeNameRE matches characters that are not alphanumeric, hyphens, or underscores
	sanitizeNameRE = regexp.MustCompile(`[^a-zA-Z0-9\-_]+`)
	// multiHyphenRE matches multiple consecutive hyphens
	multiHyphenRE = regexp.MustCompile(`-+`)
)

// SanitizeName converts a name to a valid filename/identifier
// It removes special characters, converts to lowercase, and normalizes hyphens
func SanitizeName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)

	// Replace spaces with hyphens
	name = strings.ReplaceAll(name, " ", "-")

	// Remove special characters except hyphens and underscores
	name = sanitizeNameRE.ReplaceAllString(name, "")

	// Replace multiple consecutive hyphens with a single hyphen
	name = multiHyphenRE.ReplaceAllString(name, "-")

	// Trim leading/trailing hyphens
	name = strings.Trim(name, "-")

	// Ensure not empty
	if name == "" {
		name = "unnamed"
	}

	return name
}
