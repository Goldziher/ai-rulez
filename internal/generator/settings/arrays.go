package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

// documentRel is the slash-separated path the previous run's ownership record
// is keyed by: the document relative to the base directory of the run.
func documentRel(cfg *config.Config, docPath string) string {
	if docPath == "" || cfg == nil {
		return ""
	}
	rel, err := filepath.Rel(cfg.BaseDir, docPath)
	if err != nil {
		return filepath.ToSlash(docPath)
	}
	return filepath.ToSlash(rel)
}

// readPath returns the raw value at path in the JSON or JSONC document at docPath,
// or nil when the document or the key is absent or the document cannot be parsed
// (the merge reports that itself). A strict document keeps its bytes; a JSONC one
// (comments, trailing commas, a BOM) is read through the tolerant decoder, so a
// commented config is not mistaken for an empty one.
func readPath(docPath string, path []string) json.RawMessage {
	if docPath == "" {
		return nil
	}
	data, err := os.ReadFile(docPath) //nolint:gosec // path is derived from the preset layout and the base directory
	if err != nil {
		return nil
	}
	if json.Valid(data) {
		return strictPath(data, path)
	}
	value, ok := jsonmerge.LookupTree(docTree(docPath), path)
	if !ok {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return raw
}

// strictPath walks a strict JSON document and returns the value at path, or nil.
func strictPath(data []byte, path []string) json.RawMessage {
	var node json.RawMessage = data
	for _, key := range path {
		var object map[string]json.RawMessage
		if json.Unmarshal(node, &object) != nil {
			return nil
		}
		next, ok := object[key]
		if !ok {
			return nil
		}
		node = next
	}
	return node
}

// equalJSON compares two raw JSON values structurally.
func equalJSON(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

// arrayKey builds the owned key for an array in which ai-rulez owns only the
// elements in ours and the consumer owns the rest. The written value is the
// consumer's elements (byte-for-byte, in their order) plus the elements of ours
// the array lacks, appended in declaration order; elements an earlier run added
// and this one no longer wants leave with it. Only the elements ai-rulez added or
// claimed earlier are recorded as its own, so an identical element the consumer
// wrote stays theirs on clean.
func arrayKey(cfg *config.Config, docPath string, path []string, ours []json.RawMessage) jsonmerge.OwnedKey {
	if !isJSONDocument(docPath) {
		return nativeArrayKey(cfg, docPath, path, ours)
	}
	var existing []json.RawMessage
	if raw := readPath(docPath, path); raw != nil {
		if json.Unmarshal(raw, &existing) != nil {
			existing = nil // a non-array value is the consumer's; Apply reports it
		}
	}
	// A claim records digests, not values, so what an earlier run owned is read
	// off the elements the document holds now.
	value, claimed := planElements(previousElementClaims(cfg, docPath, path), existing, ours)
	generic := make([]any, 0, len(claimed))
	for _, element := range claimed {
		var parsed any
		if json.Unmarshal(element, &parsed) == nil {
			generic = append(generic, parsed)
		}
	}
	values := make([]any, len(value))
	for i, element := range value {
		values[i] = element
	}
	return jsonmerge.OwnedKey{Path: path, Value: values, Elements: generic}
}

// previousElementClaims lists the claims an earlier run recorded for the array at path.
func previousElementClaims(cfg *config.Config, docPath string, path []string) []jsonmerge.Claim {
	var claims []jsonmerge.Claim
	for _, claim := range cfg.Run.PreviousClaims(documentRel(cfg, docPath)) {
		if equalPath(claim.Path, path) {
			claims = append(claims, claim)
		}
	}
	return claims
}

// planElements decides an array in which ai-rulez owns only some elements. The
// array keeps the consumer's elements in their order and appends the elements of
// ours it lacks; an element an earlier run claimed that ours no longer wants
// leaves, one copy per claimed copy, so a hand-written duplicate stays. claimed
// is what this run added or already owned: an identical element the consumer
// wrote is theirs and is never claimed, so clean cannot take it back. Elements
// are compared by digest, which is how a manifest records them.
func planElements[T any](previous []jsonmerge.Claim, existing, ours []T) (value, claimed []T) {
	matchers := make([]*jsonmerge.ElementMatcher, len(previous))
	for i, claim := range previous {
		matchers[i] = claim.NewElementMatcher()
	}
	wanted := make(map[string]bool, len(ours))
	for _, element := range ours {
		wanted[jsonmerge.Digest(element)] = true
	}
	present := make(map[string]bool, len(existing)+len(ours))
	retained := map[string]int{} // copies an earlier run claimed that stay
	value = make([]T, 0, len(existing)+len(ours))
	for _, element := range existing {
		sum := jsonmerge.Digest(element)
		owned := false
		for _, matcher := range matchers {
			if matcher.Take(element) {
				owned = true
				break
			}
		}
		if owned && !wanted[sum] {
			continue
		}
		value = append(value, element)
		present[sum] = true
		if owned {
			retained[sum]++
		}
	}
	for _, element := range ours {
		sum := jsonmerge.Digest(element)
		switch {
		case !present[sum]:
			present[sum] = true
			value = append(value, element)
		case retained[sum] > 0:
			retained[sum]--
		default:
			continue // identical to an element the consumer wrote: theirs
		}
		claimed = append(claimed, element)
	}
	return value, claimed
}

func equalPath(a, b []string) bool { return slices.Equal(a, b) }
