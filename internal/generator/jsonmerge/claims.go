package jsonmerge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/samber/oops"
)

// Claim records one thing ai-rulez wrote into a merged document, addressed by
// key path. A claim is the only license ai-rulez has to take something out of a
// document the consumer owns.
//
//   - Path alone: the key at Path (an MCP server entry, a scalar) is ours.
//   - Elements: the key at Path is the consumer's array and only these elements
//     of it are ours. A record read back from a manifest holds ElementSums, the
//     digests of those elements, instead: an element can carry a resolved secret
//     (an MCP server's env), so the manifest never stores the values. Elements
//     is the in-memory form a merge produces and tests build; both name
//     elements through the methods below.
//   - Equals, when set, restricts removal to a value that still equals it.
//   - Sum does the same with a digest of the value (see Digest). The record a
//     merge leaves behind carries Sum rather than Equals, because the value can
//     hold a resolved secret (a header, an env variable) that must not be copied
//     into another file.
//   - Alone restricts removal to when no other top-level key remains.
//   - Preexisting names ancestor paths of Path that were already in the document,
//     holding nothing, when ai-rulez first merged into it (a user's empty
//     [parent] table, a null "parent:"). Unmerge never removes those, only what
//     ai-rulez put under them. It is recorded only in that case, so a document
//     ai-rulez created itself leaves it empty. An ancestor that already held
//     only ai-rulez's own content when the claim was recorded (a regeneration)
//     cannot be told from one ai-rulez made, and is treated as ai-rulez's.
//
// A claim with Equals or Sum removes only the value ai-rulez wrote: a user who
// edited it has taken it over, and it stays.
type Claim struct {
	Path     []string `json:"path"`
	Elements []any    `json:"elements,omitempty"`
	// ElementSums is the persisted form of Elements: the Digest of each element.
	ElementSums []string `json:"elementSums,omitempty"`
	Equals      any      `json:"equals,omitempty"`
	Sum         string   `json:"sum,omitempty"`
	Alone       bool     `json:"alone,omitempty"`

	Preexisting [][]string `json:"preexisting,omitempty"`

	// NoFinalNewline records that the document did not end in a newline when
	// ai-rulez first merged into it (Apply adds one), so Unmerge takes the newline
	// it added back out and restores the original bytes.
	NoFinalNewline bool `json:"noFinalNewline,omitempty"`
}

// claimWire is the manifest encoding of a Claim: element digests, never values.
type claimWire struct {
	Path           []string   `json:"path"`
	Elements       []any      `json:"elements,omitempty"`
	ElementSums    []string   `json:"elementSums,omitempty"`
	Equals         any        `json:"equals,omitempty"`
	Sum            string     `json:"sum,omitempty"`
	Alone          bool       `json:"alone,omitempty"`
	Preexisting    [][]string `json:"preexisting,omitempty"`
	NoFinalNewline bool       `json:"noFinalNewline,omitempty"`
}

// MarshalJSON writes the claim with its elements as digests: a manifest records
// which elements are ours without copying their values, which can be secrets.
func (c Claim) MarshalJSON() ([]byte, error) {
	return json.Marshal(claimWire{
		Path: c.Path, ElementSums: c.ElementDigests(), Equals: c.Equals, Sum: c.Sum, Alone: c.Alone,
		Preexisting: c.Preexisting, NoFinalNewline: c.NoFinalNewline,
	})
}

// UnmarshalJSON reads a claim. A record written before elements were digested
// carries their values; they are folded into digests on the way in so no value
// stays in memory or is written back.
func (c *Claim) UnmarshalJSON(data []byte) error {
	var wire claimWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err //nolint:wrapcheck // decoding error is self-describing
	}
	*c = Claim{
		Path: wire.Path, ElementSums: wire.ElementSums, Equals: wire.Equals, Sum: wire.Sum, Alone: wire.Alone,
		Preexisting: wire.Preexisting, NoFinalNewline: wire.NoFinalNewline,
	}
	if wire.Elements != nil {
		c.ElementSums = c.digestsWith(wire.Elements)
	}
	return nil
}

// HasElements reports whether the claim covers only some elements of an array.
func (c Claim) HasElements() bool { return c.Elements != nil || c.ElementSums != nil }

// ElementDigests lists the digests of the claimed elements, sorted and distinct.
func (c Claim) ElementDigests() []string { return c.digestsWith(c.Elements) }

func (c Claim) digestsWith(values []any) []string {
	if values == nil && c.ElementSums == nil {
		return nil
	}
	sums := append([]string{}, c.ElementSums...)
	for _, v := range values {
		if sum := Digest(v); sum != "" {
			sums = append(sums, sum)
		}
	}
	sort.Strings(sums)
	return slices.Compact(sums)
}

// OwnsElement reports whether value is one of the claimed elements, compared as
// JSON the way a claim read back from a manifest is.
func (c Claim) OwnsElement(value any) bool {
	sum := Digest(value)
	return sum != "" && slices.Contains(c.ElementDigests(), sum)
}

// ownsRaw is OwnsElement for an element still in its raw JSON form.
func (c Claim) ownsRaw(raw json.RawMessage) bool {
	sum := digestRaw(raw)
	return sum != "" && slices.Contains(c.ElementDigests(), sum)
}

// ElementsIn returns the candidates the claim owns, in order. It is how a caller
// holding the document's current elements learns which of them an earlier run
// claimed, without the claim storing their values.
func (c Claim) ElementsIn(candidates []any) []any {
	var owned []any
	for _, candidate := range candidates {
		if c.OwnsElement(candidate) {
			owned = append(owned, candidate)
		}
	}
	return owned
}

// WithoutElements returns the claim minus the elements other claims.
func (c Claim) WithoutElements(other Claim) Claim {
	drop := other.ElementDigests()
	kept := []string{}
	for _, sum := range c.ElementDigests() {
		if !slices.Contains(drop, sum) {
			kept = append(kept, sum)
		}
	}
	c.Elements, c.ElementSums = nil, kept
	return c
}

// Digest is the canonical fingerprint of a JSON value: the hex SHA-256 of its
// encoding with object keys sorted, so a value and the same value read back from
// a document digest alike. It returns "" for a value that cannot be encoded.
func Digest(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return digestRaw(encoded)
}

func digestRaw(raw []byte) string {
	var normalized any
	if json.Unmarshal(raw, &normalized) != nil {
		return ""
	}
	canonical, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// Matches reports whether raw is a value this claim may remove: any value when
// the claim carries no guard, otherwise only the one recorded.
func (c Claim) Matches(raw json.RawMessage) bool {
	if c.Equals != nil && !jsonEquals(raw, c.Equals) {
		return false
	}
	return c.Sum == "" || digestRaw(raw) == c.Sum
}

// Guarded reports whether the claim restricts removal to a recorded value.
func (c Claim) Guarded() bool {
	return c.Equals != nil || c.Sum != ""
}

// claimsFor derives the ownership record of one merge from its owned keys.
func claimsFor(owned []OwnedKey) []Claim {
	var claims []Claim
	for _, key := range owned {
		segs := key.segments()
		if len(segs) == 0 || key.Remove {
			continue
		}
		switch {
		case key.Elements != nil:
			if len(key.Elements) > 0 {
				claims = append(claims, Claim{Path: segs, Elements: key.Elements})
			}
		case key.Members:
			claims = append(claims, memberClaims(segs, key.Value)...)
		default:
			claims = append(claims, Claim{Path: segs, Sum: Digest(key.Value), Alone: key.Alone})
		}
	}
	return claims
}

// memberClaims names every entry of a map value, sorted so the record is
// deterministic, each guarded by the entry's value. A map with no entries claims
// the key itself while it is still empty.
func memberClaims(segs []string, value any) []Claim {
	names, entries, ok := memberEntries(value)
	if !ok {
		return []Claim{{Path: segs, Sum: Digest(value)}}
	}
	claims := make([]Claim, 0, len(names))
	for _, name := range names {
		claims = append(claims, Claim{Path: append(append([]string{}, segs...), name), Sum: Digest(entries[name])})
	}
	return claims
}

// memberEntries splits a string-keyed map value into its sorted names and its
// entries; ok is false for anything else, and for a map with no entries.
func memberEntries(value any) (names []string, entries map[string]any, ok bool) {
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String || rv.Len() == 0 {
		return nil, nil, false
	}
	entries = make(map[string]any, rv.Len())
	for _, k := range rv.MapKeys() {
		names = append(names, k.String())
		entries[k.String()] = rv.MapIndex(k).Interface()
	}
	sort.Strings(names)
	return names, entries, true
}

// Unmerged is the outcome of Unmerge.
type Unmerged struct {
	// Body is the document without the claimed content; set when Changed.
	Body string
	// Changed is false when nothing claimed was present.
	Changed bool
	// Empty is true when no member is left, so the file held only ai-rulez's
	// content and may be deleted.
	Empty bool
	// Kept lists the claimed paths left in place because their value is no
	// longer the one ai-rulez wrote (the user edited it).
	Kept [][]string
}

// Unmerge removes the claimed content from the document at path, leaving every
// other member and the document's formatting as Apply does. Claimed content that
// is absent, or whose container is not what the claim expects, is skipped. A
// missing file changes nothing.
func Unmerge(path string, claims []Claim) (Unmerged, error) {
	existing, found, err := readExistingDocument(path)
	if err != nil || !found {
		return Unmerged{}, err
	}
	return UnmergeDocument(path, existing, claims)
}

// UnmergeDocument is Unmerge for a document already read; path only names it in
// errors.
func UnmergeDocument(path, existing string, claims []Claim) (Unmerged, error) {
	bom, existing := SplitBOM(existing)
	members, err := decodeObjectMembers([]byte(existing))
	if err != nil {
		result, err := unmergeJSONC(path, existing, claims, err)
		if result.Changed && !result.Empty {
			result.Body = bom + result.Body
		}
		return result, err
	}

	indent := detectTopLevelIndent(existing)
	newline := detectLineEnding(existing)
	members, changed, kept, err := unmergeAll(members, claims, indent, newline)
	if err != nil {
		return Unmerged{}, oops.With("path", path).Wrapf(err, "remove ai-rulez content from JSON settings document")
	}
	if !changed {
		return Unmerged{Kept: kept}, nil
	}
	rendered, err := renderMembers(members, 1, indent, newline)
	if err != nil {
		return Unmerged{}, oops.With("path", path).Wrapf(err, "encode JSON settings document")
	}
	body := RestoreFinalNewline(claims, rendered+newline)
	if err := checkPreservedDocuments(existing, body, func(before, after map[string]any) error {
		return CheckPreservedUnmerge(before, after, claims)
	}); err != nil {
		return Unmerged{}, oops.With("path", path).
			Hint("ai-rulez could not remove its keys from this JSON document without altering the rest of it; the file was left untouched").
			Wrapf(err, "unmerged JSON settings document does not preserve the existing content")
	}
	empty := len(members) == 0
	if !empty {
		body = bom + body
	}
	return Unmerged{Body: body, Changed: true, Empty: empty, Kept: kept}, nil
}

// unmergeAll applies the claims in order, the Alone ones last and only while a
// single top-level member remains. It also returns the claimed paths that were
// left because their value is not the recorded one and are still present once
// every claim has run (a later claim may match what an earlier one did not).
func unmergeAll(members []jsonMember, claims []Claim, indent, newline string,
) (result []jsonMember, changed bool, kept [][]string, err error) {
	var mismatched [][]string
	for _, alone := range []bool{false, true} {
		for _, claim := range claims {
			if len(claim.Path) == 0 || claim.Alone != alone || (alone && len(members) != 1) {
				continue
			}
			next, did, mismatch, err := unmergeClaim(members, claim.Path, claim, 1, indent, newline)
			if err != nil {
				return nil, false, nil, err
			}
			members, changed = next, changed || did
			if mismatch {
				mismatched = append(mismatched, claim.Path)
			}
		}
	}
	seen := map[string]bool{}
	for _, path := range mismatched {
		key := strings.Join(path, "\x00")
		if !seen[key] && memberPresent(members, path) {
			seen[key] = true
			kept = append(kept, path)
		}
	}
	return members, changed, kept, nil
}

// memberPresent reports whether the key path addresses a member of the document.
func memberPresent(members []jsonMember, path []string) bool {
	for _, member := range members {
		if member.Key != path[0] {
			continue
		}
		if len(path) == 1 {
			return true
		}
		raw := bytes.TrimSpace(member.Raw)
		if len(raw) == 0 || raw[0] != '{' {
			return false
		}
		child, err := decodeObjectMembers(raw)
		return err == nil && memberPresent(child, path[1:])
	}
	return false
}

// unmergeClaim removes the member (or array elements) the claim addresses,
// dropping an ancestor object the removal leaves empty. mismatch reports a member
// found at the path whose value the claim's guard rejected.
func unmergeClaim(members []jsonMember, path []string, claim Claim, depth int, indent, newline string,
) (result []jsonMember, changed, mismatch bool, err error) {
	head, rest := path[0], path[1:]
	idx := -1
	for i, member := range members {
		if member.Key == head {
			idx = i
			break
		}
	}
	if idx < 0 {
		return members, false, false, nil
	}

	if len(rest) == 0 {
		return unmergeLeaf(members, idx, claim, depth, indent, newline)
	}
	raw := bytes.TrimSpace(members[idx].Raw)
	if len(raw) == 0 || raw[0] != '{' {
		return members, false, false, nil
	}
	child, err := decodeObjectMembers(raw)
	if err != nil {
		return nil, false, false, err
	}
	child, did, mismatch, err := unmergeClaim(child, rest, claim, depth+1, indent, newline)
	if err != nil || !did {
		return members, false, mismatch, err
	}
	if len(child) == 0 {
		return removeMember(members, head), true, false, nil
	}
	rendered, err := renderMembers(child, depth+1, indent, newline)
	if err != nil {
		return nil, false, false, err
	}
	members[idx] = jsonMember{Key: head, Raw: []byte(rendered)}
	return members, true, false, nil
}

// unmergeLeaf applies a claim to the member at members[idx].
func unmergeLeaf(members []jsonMember, idx int, claim Claim, depth int, indent, newline string,
) (result []jsonMember, changed, mismatch bool, err error) {
	head := members[idx].Key
	if !claim.Matches(members[idx].Raw) {
		return members, false, true, nil
	}
	if !claim.HasElements() {
		return removeMember(members, head), true, false, nil
	}

	var elements []json.RawMessage
	if json.Unmarshal(members[idx].Raw, &elements) != nil {
		return members, false, false, nil //nolint:nilerr // not an array: the consumer's value, left alone
	}
	kept := make([]json.RawMessage, 0, len(elements))
	for _, element := range elements {
		if !claim.ownsRaw(element) {
			kept = append(kept, element)
		}
	}
	if len(kept) == len(elements) {
		return members, false, false, nil
	}
	if len(kept) == 0 {
		return removeMember(members, head), true, false, nil
	}
	rendered, err := replaceMemberValue(members, head, kept, depth, indent, newline)
	return rendered, true, false, err
}

// anyEquals reports whether raw equals any of the candidates.
func anyEquals(raw json.RawMessage, candidates []any) bool {
	for _, candidate := range candidates {
		if jsonEquals(raw, candidate) {
			return true
		}
	}
	return false
}

// jsonEquals compares a raw JSON value to a Go value structurally, so a claim
// read back from a manifest equals the value it was recorded from.
func jsonEquals(raw json.RawMessage, want any) bool {
	var got, wanted any
	if json.Unmarshal(raw, &got) != nil {
		return false
	}
	encoded, err := json.Marshal(want)
	if err != nil || json.Unmarshal(encoded, &wanted) != nil {
		return false
	}
	return reflect.DeepEqual(got, wanted)
}

// parseFailure builds the error for a document that is neither strict JSON nor
// JSONC with an object root.
func parseFailure(path string, cause error) error {
	return oops.
		With("path", path).
		Hint(fmt.Sprintf(
			"%s is not parseable JSON, and ai-rulez will not overwrite a file it cannot merge into. "+
				"Fix the syntax, or move the file aside.", path)).
		Wrapf(cause, "parse existing JSON settings document")
}
