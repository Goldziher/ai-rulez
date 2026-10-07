package providers

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

// ElementsSpec makes a generic sidecar also own some elements of an array
// member of its JSON or JSONC document (Kilo's `instructions` list of rule
// globs): ai-rulez adds each value once, and every other element is the
// consumer's.
type ElementsSpec struct {
	// Key is the path of the array member, as in SidecarSpec.Key.
	Key []string `toml:"key" yaml:"key" json:"key"`
	// Values are the elements ai-rulez adds.
	Values []string `toml:"values" yaml:"values" json:"values"`
	// ProjectOnly marks values that are project-relative globs: they are left out
	// of the user-scope document, where they would resolve against no project.
	ProjectOnly bool `toml:"project_only,omitempty" yaml:"project_only,omitempty" json:"project_only,omitempty"`
}

// validateElements checks the elements block of the sidecar at index i.
func validateElements(i int, sc *SidecarSpec) error {
	e := sc.Elements
	if e == nil {
		return nil
	}
	if sc.DocFormat() != DocFormatJSON && sc.DocFormat() != DocFormatJSONC {
		return fmt.Errorf("sidecars[%d].elements is only valid on a json or jsonc document", i)
	}
	if len(e.Key) == 0 || len(e.Values) == 0 {
		return fmt.Errorf("sidecars[%d].elements needs a key and at least one value", i)
	}
	for _, s := range append(slices.Clone(e.Key), e.Values...) {
		if s == "" {
			return fmt.Errorf("sidecars[%d].elements: key segments and values must not be empty", i)
		}
	}
	return nil
}

// elementsOwnedKey returns the owned array key for the sidecar's elements: the
// document's existing array with the missing values appended, claiming the
// values ai-rulez added now or recorded as its own in the previous run. ok is
// false when the member exists but is not an array, which is the consumer's to
// fix and is left alone.
func elementsOwnedKey(sc *SidecarSpec, cfg *config.Config, outputPath string) (key jsonmerge.OwnedKey, ok bool, err error) {
	e := sc.Elements
	existing := []any{}
	if doc, found, rerr := jsonmerge.ReadExistingWith(cfg.ReadExisting, outputPath); rerr != nil {
		return key, false, rerr
	} else if found {
		if tree, derr := jsonmerge.DecodeTolerantTree(doc); derr == nil {
			if v, present := jsonmerge.LookupTree(tree, e.Key); present {
				arr, isArray := v.([]any)
				if !isArray {
					return key, false, nil
				}
				existing = arr
			}
		}
		// A document that does not parse is reported by the merge itself.
	}

	ours := make([]any, len(e.Values))
	for i, v := range e.Values {
		ours[i] = v
	}
	owns := func(any) bool { return false }
	if cfg != nil && cfg.Run != nil {
		rel := projectRelativePath(cfg, outputPath)
		var previous []jsonmerge.Claim
		for _, claim := range cfg.Run.PreviousClaims(rel) {
			if slices.Equal(claim.Path, e.Key) {
				previous = append(previous, claim)
			}
		}
		owns = jsonmerge.ClaimsOwner[any](previous)
		if cfg.Run.WasGenerated(rel) {
			// A document ai-rulez wrote whole holds only its own values.
			left := map[string]int{}
			for _, v := range ours {
				left[jsonmerge.Digest(v)]++
			}
			claimsOwn := owns
			owns = func(element any) bool {
				if sum := jsonmerge.Digest(element); left[sum] > 0 {
					left[sum]--
					return true
				}
				return claimsOwn(element)
			}
		}
	}
	// Decided with the shared planner so a value identical to one the consumer
	// wrote is never claimed (clean would otherwise delete their entry).
	entries, claimed := jsonmerge.PlanElements(owns, existing, ours)
	if claimed == nil {
		claimed = []any{}
	}
	return jsonmerge.OwnedKey{Path: e.Key, Value: entries, Elements: claimed}, true, nil
}

func projectRelativePath(cfg *config.Config, path string) string {
	root := cfg.BaseDir
	if cfg.Run != nil && cfg.Run.Scope != nil && cfg.Run.Scope.RootDir != "" {
		root = cfg.Run.Scope.RootDir
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
