package yamlmerge

import (
	"slices"

	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/samber/oops"
)

// Unmerge removes the claimed content from the YAML document at path, leaving
// every other member as Apply does. Claimed content that is absent, or whose
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
	before, err := parse(existing)
	if err != nil {
		return Unmerged{}, parseFailure(path, err)
	}
	ed := newEditor(existing)

	changed, mismatched, err := ed.removeClaims(path, claims)
	if err != nil {
		return Unmerged{}, err
	}

	if changed {
		ed.src = jsonmerge.RestoreFinalNewline(claims, ed.src)
	}
	after, err := parse(ed.src)
	if err != nil {
		return Unmerged{}, oops.With("path", path).Wrapf(err, "removing ai-rulez content broke the YAML document")
	}
	if err := jsonmerge.CheckPreservedUnmerge(before.tree, after.tree, claims); err != nil {
		return Unmerged{}, oops.
			With("path", path).
			Hint("ai-rulez could not remove its keys from this YAML document without altering the rest of it; the file was left untouched").
			Wrapf(err, "unmerged YAML document does not preserve the existing content")
	}
	var kept [][]string
	for _, claimPath := range mismatched {
		if _, ok := jsonmerge.LookupTree(after.tree, claimPath); ok && !slices.ContainsFunc(kept, func(p []string) bool {
			return slices.Equal(p, claimPath)
		}) {
			kept = append(kept, claimPath)
		}
	}
	if !changed {
		return Unmerged{Kept: kept}, nil
	}
	empty := len(after.tree) == 0 && !after.hasComment
	body := bom + ed.src
	if empty {
		body = ""
	}
	return Unmerged{Body: body, Changed: true, Empty: empty, Kept: kept}, nil
}

// unmergeClaim removes what one claim addresses.
func (e *editor) unmergeClaim(claim Claim) (changed, mismatch bool, err error) {
	p, err := parse(e.src)
	if err != nil {
		return false, false, err
	}
	if claim.Alone && len(p.tree) != 1 {
		return false, false, nil
	}
	value, ok := jsonmerge.LookupTree(p.tree, claim.Path)
	if !ok {
		return false, false, nil
	}
	if !claim.MatchesValue(value) {
		return false, true, nil
	}
	if claim.Elements == nil {
		return true, false, e.remove(claim.Path, claim)
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

func (e *editor) removeClaims(path string, claims []Claim) (changed bool, mismatched [][]string, err error) {
	changed = false
	for _, alone := range []bool{false, true} {
		for _, claim := range claims {
			if len(claim.Path) == 0 || claim.Alone != alone {
				continue
			}
			did, mismatch, err := e.unmergeClaim(claim)
			if err != nil {
				return false, nil, oops.With("path", path).Wrapf(err, "remove ai-rulez content from YAML document")
			}
			changed = changed || did
			if mismatch {
				mismatched = append(mismatched, claim.Path)
			}
		}
	}
	return changed, mismatched, nil
}
