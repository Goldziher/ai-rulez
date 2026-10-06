package tagresolve

import "sort"

// FileChange is one file that differs between two trees.
type FileChange struct {
	Path string `json:"path"`
	// Change is "A" (added), "M" (modified) or "D" (deleted).
	Change string `json:"change"`
}

// DiffFiles compares two trees given as path -> content hash.
func DiffFiles(oldHashes, newHashes map[string]string) []FileChange {
	var out []FileChange
	for p, h := range newHashes {
		switch old, ok := oldHashes[p]; {
		case !ok:
			out = append(out, FileChange{p, "A"})
		case old != h:
			out = append(out, FileChange{p, "M"})
		}
	}
	for p := range oldHashes {
		if _, ok := newHashes[p]; !ok {
			out = append(out, FileChange{p, "D"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
