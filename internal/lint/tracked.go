package lint

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// Tracked reports whether rel (slash path from the top) is in the git index. It
// is false for every path when the tree was not read from git.
func (t *Tree) Tracked(rel string) bool {
	if !t.Git {
		return false
	}
	_, ok := t.files[filepath.ToSlash(rel)]
	return ok
}

// DropUntracked removes the findings that sit in an existing file of the tree
// that git does not track and keep does not name, and returns how many it
// removed. Paths are slash paths relative to the tree top. A finding without a
// file, in a file that is not on disk (a synthetic location) or outside the
// tree stays, and so does every finding when the tree was not read from git,
// where "untracked" has no meaning.
func (r *Report) DropUntracked(tree *Tree, keep map[string]bool) int {
	if tree == nil || !tree.Git {
		return 0
	}
	kept := r.Findings[:0]
	dropped := 0
	for i := range r.Findings {
		f := r.Findings[i]
		rel := f.RepoPath()
		untracked := rel != "" && rel != "." && !filepath.IsAbs(rel) && !safefs.RelEscapes(rel) &&
			!tree.Tracked(rel) && !keep[rel] && tree.Exists(rel)
		if untracked {
			dropped++
			continue
		}
		kept = append(kept, f)
	}
	r.Findings = kept
	return dropped
}
