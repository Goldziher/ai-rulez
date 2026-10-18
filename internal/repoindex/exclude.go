package repoindex

import (
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/verifiers/vspec"
)

// dirProbe is the name of a child no glob names, used to ask whether a whole
// directory is excluded.
const dirProbe = "\x00"

// Excluder decides which project-relative paths are left out by a list of
// globs. A glob is relative to the project root, `**` crosses directories, `*`
// and `?` do not, `{a,b}` alternates, a pattern without a slash matches at any
// depth and a trailing slash names a directory. A nil Excluder excludes nothing.
type Excluder struct {
	globs []vspec.Glob
}

// NewExcluder compiles the globs; an invalid one is an error.
func NewExcluder(globs []string) (*Excluder, error) {
	if len(globs) == 0 {
		return nil, nil //nolint:nilnil // no globs, no excluder
	}
	e := &Excluder{globs: make([]vspec.Glob, 0, len(globs))}
	for _, raw := range globs {
		g, err := vspec.CompileGlob(strings.ReplaceAll(raw, "\\", "/"))
		if err != nil {
			return nil, oops.Wrapf(err, "invalid exclude glob %q", raw)
		}
		e.globs = append(e.globs, g)
	}
	return e, nil
}

// Excluded reports whether a glob matches the slash-separated path or one of
// the directories above it.
func (e *Excluder) Excluded(rel string) bool {
	if e == nil {
		return false
	}
	for p := rel; p != RootScope && p != ""; p = parentOf(p) {
		for _, g := range e.globs {
			if g.Match(p) {
				return true
			}
		}
	}
	return false
}

// ExcludedDir reports whether everything below a directory is excluded, so a
// walker need not enter it.
func (e *Excluder) ExcludedDir(rel string) bool {
	if e == nil {
		return false
	}
	return e.Excluded(strings.TrimSuffix(rel, "/") + "/" + dirProbe)
}
