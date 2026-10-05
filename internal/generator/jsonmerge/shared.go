package jsonmerge

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"sort"
	"strings"
)

// The helpers below let the TOML and YAML engines (tomlmerge, yamlmerge) share
// this package's contract: the same OwnedKey, Result, Claim and Unmerged types,
// the same ownership record and the same Members semantics, so a claim recorded
// for one format is read back by the same code as any other.

// Segments returns the key path the OwnedKey addresses: Path when set, else the
// single Name, else nil.
func (k OwnedKey) Segments() []string {
	return k.segments()
}

// ClaimsFor derives the ownership record of one merge from its owned keys.
func ClaimsFor(owned []OwnedKey) []Claim {
	return claimsFor(owned)
}

// OwnedPaths lists the key path of every owned key, expanding a Members key into
// one path per entry written.
func OwnedPaths(owned []OwnedKey) [][]string {
	return ownedPaths(owned)
}

// MemberEntries splits a string-keyed map value into its sorted names and its
// entries; ok is false for anything else, and for a map with no entries.
func MemberEntries(value any) (names []string, entries map[string]any, ok bool) {
	return memberEntries(value)
}

// ReadExisting reads the document at path. A missing, unnamed or all-whitespace
// file reports found == false; any other read failure is an error.
func ReadExisting(path string) (contents string, found bool, err error) {
	return readExistingDocument(path)
}

// MatchesValue reports whether value (a document value decoded into Go types) is
// one this claim may remove.
func (c Claim) MatchesValue(value any) bool {
	raw, err := json.Marshal(value)
	if err != nil {
		return false
	}
	return c.Matches(raw)
}

// ElementsContain reports whether value equals any of the candidates, compared
// structurally the way a claim read back from a manifest is.
func ElementsContain(value any, candidates []any) bool {
	raw, err := json.Marshal(value)
	if err != nil {
		return false
	}
	return anyEquals(raw, candidates)
}

// HasUnownedTree reports whether a decoded document holds a key outside the
// owned paths, which can only be the consumer's. A key owned as a whole counts as
// owned; one owned only through deeper paths is inspected recursively.
func HasUnownedTree(tree map[string]any, paths [][]string) bool {
	roots := make(map[string][][]string, len(paths))
	for _, path := range paths {
		roots[path[0]] = append(roots[path[0]], path[1:])
	}
	for name, value := range tree {
		rems, ok := roots[name]
		if !ok {
			return true
		}
		whole := false
		for _, rem := range rems {
			if len(rem) == 0 {
				whole = true
				break
			}
		}
		if whole {
			continue
		}
		child, isTable := value.(map[string]any)
		if !isTable || HasUnownedTree(child, rems) {
			return true
		}
	}
	return false
}

// LookupTree returns the value at path in a decoded document.
func LookupTree(tree map[string]any, path []string) (any, bool) {
	var node any = tree
	for _, part := range path {
		table, ok := node.(map[string]any)
		if !ok {
			return nil, false
		}
		if node, ok = table[part]; !ok {
			return nil, false
		}
	}
	return node, true
}

// BOM is the UTF-8 byte order mark some editors put at the start of a file.
const BOM = "\xef\xbb\xbf"

// SplitBOM separates a leading UTF-8 byte order mark from a document, so the
// engines parse the text and put the mark back on what they return.
func SplitBOM(doc string) (bom, rest string) {
	if strings.HasPrefix(doc, BOM) {
		return BOM, doc[len(BOM):]
	}
	return "", doc
}

// pathSpec is one thing a merge may change: the whole value at path, or, with
// elements, only those elements of the array there.
type pathSpec struct {
	path     []string
	elements []any
}

// ownedSpecs lists what Apply of owned is allowed to change in before.
func ownedSpecs(before map[string]any, owned []OwnedKey) []pathSpec {
	var specs []pathSpec
	for _, key := range owned {
		segs := key.segments()
		if len(segs) == 0 {
			continue
		}
		switch {
		case key.Remove:
			specs = append(specs, pathSpec{path: segs})
		case key.Elements != nil:
			// Apply writes the Value the caller computed, which may drop elements an
			// earlier run claimed and this one no longer wants, so the whole array is
			// the caller's to get right (see docmerge.Apply); only Unmerge, which the
			// engine decides for itself, is held to the claimed elements.
			specs = append(specs, pathSpec{path: segs})
		case key.Members:
			names, _, ok := memberEntries(key.Value)
			if ok {
				for _, name := range names {
					specs = append(specs, pathSpec{path: append(append([]string{}, segs...), name)})
				}
			} else if _, present := LookupTree(before, segs); !present {
				specs = append(specs, pathSpec{path: segs})
			}
		default:
			specs = append(specs, pathSpec{path: segs})
		}
	}
	return specs
}

// claimSpecs lists what Unmerge of claims is allowed to change.
func claimSpecs(claims []Claim) []pathSpec {
	specs := make([]pathSpec, 0, len(claims))
	for _, claim := range claims {
		if len(claim.Path) > 0 {
			specs = append(specs, pathSpec{path: claim.Path, elements: claim.Elements})
		}
	}
	return specs
}

// CheckPreservedApply verifies that merging owned into before, which produced
// after (both decoded documents), changed nothing but the owned keys: with every
// owned path removed from both, what is left must be identical. It is the
// engines' last line of defence against a text edit that damaged content it did
// not own, and a violation means the merge must be abandoned.
func CheckPreservedApply(before, after map[string]any, owned []OwnedKey) error {
	return checkPreserved(before, after, ownedSpecs(before, owned))
}

// CheckPreservedUnmerge is CheckPreservedApply for removing claims.
func CheckPreservedUnmerge(before, after map[string]any, claims []Claim) error {
	return checkPreserved(before, after, claimSpecs(claims))
}

func checkPreserved(before, after map[string]any, specs []pathSpec) error {
	b := deepCopy(before).(map[string]any)
	a := deepCopy(after).(map[string]any)
	for _, spec := range specs {
		spec.strip(b)
		spec.strip(a)
	}
	if diff, differs := firstDiff(b, a, nil); differs {
		return fmt.Errorf("the edit changed %s, which ai-rulez does not own", describePath(diff))
	}
	return nil
}

func describePath(path []string) string {
	if len(path) == 0 {
		return "the document"
	}
	return strings.Join(path, ".")
}

// strip removes what the spec addresses from tree, then any ancestor that is
// left holding nothing (empty or null): Apply and Unmerge drop or leave those
// either way, which is not a change to what the consumer owns.
func (s pathSpec) strip(tree map[string]any) {
	if parent := tableAt(tree, s.path[:len(s.path)-1]); parent != nil {
		last := s.path[len(s.path)-1]
		switch value, present := parent[last]; {
		case !present:
		case s.elements == nil:
			delete(parent, last)
		default:
			// A single value stands for a one-element array: an engine may turn a
			// user's string into a list to add ours beside it, and back.
			array, ok := value.([]any)
			if !ok {
				array = []any{value}
			}
			remaining := make([]any, 0, len(array))
			for _, element := range array {
				if !ElementsContain(element, s.elements) {
					remaining = append(remaining, element)
				}
			}
			if len(remaining) > 0 {
				parent[last] = remaining
				return
			}
			delete(parent, last)
		}
	}
	for n := len(s.path) - 1; n >= 1; n-- {
		parent := tableAt(tree, s.path[:n-1])
		if parent == nil {
			continue
		}
		value, present := parent[s.path[n-1]]
		if table, isTable := value.(map[string]any); present && (value == nil || isTable && len(table) == 0) {
			delete(parent, s.path[n-1])
		}
	}
}

// tableAt returns the table at path in tree, or nil.
func tableAt(tree map[string]any, path []string) map[string]any {
	node := tree
	for _, part := range path {
		next, ok := node[part].(map[string]any)
		if !ok {
			return nil
		}
		node = next
	}
	return node
}

func deepCopy(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = deepCopy(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = deepCopy(item)
		}
		return out
	default:
		return value
	}
}

// firstDiff returns the path of the first difference between two decoded values.
func firstDiff(a, b any, path []string) ([]string, bool) {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return path, true
		}
		keys := make([]string, 0, len(av)+len(bv))
		for k := range av {
			keys = append(keys, k)
		}
		for k := range bv {
			if _, in := av[k]; !in {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			x, inA := av[k]
			y, inB := bv[k]
			sub := append(slices.Clone(path), k)
			if inA != inB {
				return sub, true
			}
			if diff, differs := firstDiff(x, y, sub); differs {
				return diff, true
			}
		}
		return nil, false
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return path, true
		}
		for i := range av {
			if diff, differs := firstDiff(av[i], bv[i], path); differs {
				return diff, true
			}
		}
		return nil, false
	case float64:
		bv, ok := b.(float64)
		if ok && math.IsNaN(av) && math.IsNaN(bv) {
			return nil, false
		}
	}
	if !reflect.DeepEqual(a, b) {
		return path, true
	}
	return nil, false
}

// AnnotateClaims records, on each claim, the ancestors of its path that the
// document already held empty before the merge (see Claim.Preexisting) and
// whether it lacked a final newline (see Claim.NoFinalNewline). before is the
// document as decoded before Apply ran and original its text.
func AnnotateClaims(claims []Claim, before map[string]any, original string) []Claim {
	claims = NoteFinalNewline(claims, original)
	for i, claim := range claims {
		for n := 1; n < len(claim.Path); n++ {
			value, present := LookupTree(before, claim.Path[:n])
			if !present {
				continue
			}
			if table, isTable := value.(map[string]any); value != nil && (!isTable || len(table) > 0) {
				continue
			}
			claims[i].Preexisting = append(claims[i].Preexisting, slices.Clone(claim.Path[:n]))
		}
	}
	return claims
}

// NoteFinalNewline records on each claim whether the original document lacked a
// final newline (see Claim.NoFinalNewline).
func NoteFinalNewline(claims []Claim, original string) []Claim {
	noEOL := original != "" && !strings.HasSuffix(original, "\n")
	for i := range claims {
		claims[i].NoFinalNewline = noEOL
	}
	return claims
}

// RestoreFinalNewline takes back the final newline Apply added to a document that
// had none, when the claims record that it did not.
func RestoreFinalNewline(claims []Claim, result string) string {
	if !slices.ContainsFunc(claims, func(c Claim) bool { return c.NoFinalNewline }) {
		return result
	}
	if trimmed, ok := strings.CutSuffix(result, "\r\n"); ok {
		return trimmed
	}
	return strings.TrimSuffix(result, "\n")
}

// IsPreexisting reports whether the claim recorded path as an ancestor that was
// already in the document.
func (c Claim) IsPreexisting(path []string) bool {
	return slices.ContainsFunc(c.Preexisting, func(p []string) bool { return slices.Equal(p, path) })
}

// VerifyOwned checks that every owned value is in the decoded document as
// written and every removed key is gone.
func VerifyOwned(tree map[string]any, owned []OwnedKey) error {
	for _, key := range owned {
		segs := key.segments()
		if len(segs) == 0 {
			continue
		}
		got, present := LookupTree(tree, segs)
		switch {
		case key.Remove:
			if present {
				return fmt.Errorf("%s was not removed", strings.Join(segs, "."))
			}
		case key.Members:
			names, entries, ok := memberEntries(key.Value)
			if !ok {
				continue
			}
			for _, name := range names {
				path := append(append([]string{}, segs...), name)
				if got, present = LookupTree(tree, path); !present || !sameValue(got, entries[name]) {
					return fmt.Errorf("%s does not hold the owned value", strings.Join(path, "."))
				}
			}
		default:
			if !present || !sameValue(got, key.Value) {
				return fmt.Errorf("%s does not hold the owned value", strings.Join(segs, "."))
			}
		}
	}
	return nil
}

// sameValue compares a decoded value with the Go value that was written, by
// their JSON forms.
func sameValue(got, want any) bool {
	return Digest(got) != "" && Digest(got) == Digest(want)
}

// HasUserElements reports whether a decoded document holds, in an array of which
// ai-rulez owns only some Elements, an element that is not among them. Such an
// array is shared with its consumer even when every key in the document is owned.
func HasUserElements(tree map[string]any, owned []OwnedKey) bool {
	for _, key := range owned {
		segs := key.segments()
		if len(segs) == 0 || key.Remove || len(key.Elements) == 0 {
			continue
		}
		value, ok := LookupTree(tree, segs)
		if !ok {
			continue
		}
		array, isArray := value.([]any)
		if !isArray {
			return true
		}
		for _, element := range array {
			if !ElementsContain(element, key.Elements) {
				return true
			}
		}
	}
	return false
}

// DecodeTree decodes a strict JSON document into generic Go values, for the
// preservation check.
func DecodeTree(doc string) (map[string]any, error) {
	tree := map[string]any{}
	if err := json.Unmarshal([]byte(doc), &tree); err != nil {
		return nil, err
	}
	return tree, nil
}

// ValidateOwned rejects owned keys whose fields contradict each other: Members
// (merge a map entry by entry) and Elements (claim some elements of an array)
// describe different shapes of value and cannot both be set.
func ValidateOwned(owned []OwnedKey) error {
	for _, key := range owned {
		if key.Members && key.Elements != nil {
			return fmt.Errorf("owned key %q sets both Members and Elements, which describe different kinds of value",
				strings.Join(key.segments(), "."))
		}
	}
	return nil
}
