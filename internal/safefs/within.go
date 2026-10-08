package safefs

import (
	"path/filepath"
	"strings"
)

// RelEscapes reports whether rel, a path produced by filepath.Rel, leaves the
// base it was computed against: it is ".." or starts with "../" (or "..\" on
// Windows; a slash-form path is recognized on every platform). A name that
// merely begins with two dots ("..foo", "...") is an ordinary child.
func RelEscapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Within reports whether target is root itself or lies below it, comparing the
// paths lexically (no symlink resolution). Both must be of the same kind
// (both absolute or both relative to the same directory); a pair filepath.Rel
// cannot relate, such as different volumes, is not within.
func Within(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && !RelEscapes(rel)
}
