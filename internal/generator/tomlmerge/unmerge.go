package tomlmerge

import (
	"slices"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/samber/oops"
)

// Unmerge removes the claimed content from the TOML document at path, leaving
// every other statement as Apply does. Claimed content that is absent, or whose
// value is no longer the one ai-rulez wrote, is skipped. A missing file changes
// nothing.
func Unmerge(path string, claims []Claim) (Unmerged, error) {
	existing, found, err := jsonmerge.ReadExisting(path)
	if err != nil || !found {
		return Unmerged{}, err
	}
	return UnmergeDocument(path, existing, claims)
}

// UnmergeDocument is Unmerge for a document already read; path only names it in
// errors.
func UnmergeDocument(path, existing string, claims []Claim) (Unmerged, error) {
	bom, existing := jsonmerge.SplitBOM(existing)
	before, err := parseTree(existing)
	if err != nil {
		return Unmerged{}, parseFailure(path, err)
	}
	ed := &editor{src: existing, newline: detectLineEnding(existing)}

	changed := false
	var mismatched [][]string
	for _, alone := range []bool{false, true} {
		for _, claim := range claims {
			if len(claim.Path) == 0 || claim.Alone != alone {
				continue
			}
			did, mismatch, err := ed.unmergeClaim(claim)
			if err != nil {
				return Unmerged{}, oops.With("path", path).Wrapf(err, "remove ai-rulez content from TOML document")
			}
			changed = changed || did
			if mismatch {
				mismatched = append(mismatched, claim.Path)
			}
		}
	}

	if changed {
		ed.src = jsonmerge.RestoreFinalNewline(claims, ed.src)
	}
	tree, err := parseTree(ed.src)
	if err != nil {
		return Unmerged{}, oops.With("path", path).Wrapf(err, "removing ai-rulez content broke the TOML document")
	}
	if err := jsonmerge.CheckPreservedUnmerge(before, tree, claims); err != nil {
		return Unmerged{}, oops.
			With("path", path).
			Hint("ai-rulez could not remove its keys from this TOML document without altering the rest of it; the file was left untouched").
			Wrapf(err, "unmerged TOML document does not preserve the existing content")
	}
	var kept [][]string
	for _, claimPath := range mismatched {
		if _, ok := jsonmerge.LookupTree(tree, claimPath); ok {
			kept = appendUnique(kept, claimPath)
		}
	}
	if !changed {
		return Unmerged{Kept: kept}, nil
	}
	doc, err := scan(ed.src)
	if err != nil {
		return Unmerged{}, oops.With("path", path).Wrapf(err, "scan TOML document")
	}
	empty := len(tree) == 0 && !doc.hasComment
	body := bom + ed.src
	if empty {
		body = ""
	}
	return Unmerged{Body: body, Changed: true, Empty: empty, Kept: kept}, nil
}

func appendUnique(paths [][]string, path []string) [][]string {
	for _, existing := range paths {
		if slices.Equal(existing, path) {
			return paths
		}
	}
	return append(paths, path)
}

// unmergeClaim removes what one claim addresses.
func (e *editor) unmergeClaim(claim Claim) (changed, mismatch bool, err error) {
	tree, err := parseTree(e.src)
	if err != nil {
		return false, false, err
	}
	if claim.Alone && len(tree) != 1 {
		return false, false, nil
	}
	value, ok := jsonmerge.LookupTree(tree, claim.Path)
	if !ok {
		return false, false, nil
	}
	doc, err := e.scan()
	if err != nil {
		return false, false, err
	}
	if _, _, blocked := doc.blockedAncestor(claim.Path); blocked {
		// The key lives inside an inline table or one element of an array of tables the
		// user wrote; there is no statement of its own to remove, and it is theirs to edit.
		return false, true, nil
	}
	if !claim.MatchesValue(value) {
		return false, true, nil
	}
	if claim.Elements == nil {
		before := e.src
		err := e.remove(claim.Path, claim)
		return e.src != before, false, err
	}

	array, isArray := value.([]any)
	if !isArray {
		return false, false, nil
	}
	remaining := make([]any, 0, len(array))
	for _, element := range array {
		if !jsonmerge.ElementsContain(element, claim.Elements) {
			remaining = append(remaining, element)
		}
	}
	switch {
	case len(remaining) == len(array):
		return false, false, nil
	case len(remaining) == 0:
		return true, false, e.remove(claim.Path, claim)
	default:
		return true, false, e.set(claim.Path, remaining)
	}
}
