// Package rulefiles holds the rendering primitives shared by the hand-written
// presets and the provider DSL generator: stable item identifiers and the
// inline "## Rules" / "## Context" sections.
package rulefiles

import "strings"

// ID converts a content name into a filesystem-safe identifier: spaces,
// underscores and path separators become dashes, every other character outside
// [A-Za-z0-9-] is dropped, and surrounding dashes are trimmed.
func ID(name string) string {
	replacer := strings.NewReplacer(
		" ", "-",
		"_", "-",
		"/", "-",
		"\\", "-",
	)
	sanitized := replacer.Replace(name)
	var builder strings.Builder
	for _, r := range sanitized {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			builder.WriteRune(r)
		}
	}
	return strings.Trim(builder.String(), "-")
}
