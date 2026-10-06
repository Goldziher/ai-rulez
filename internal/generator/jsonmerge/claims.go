package jsonmerge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"slices"
	"sort"

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
//   - Equals, when set, restricts removal to a value that still equals it. It is
//     an in-memory convenience: a claim written to or read from a manifest holds
//     the digest in Sum instead (see textClaim for the one exception).
//   - Sum does the same with a digest of the value (see Digest). The record a
//     merge leaves behind carries Sum rather than Equals, because the value can
//     hold a resolved secret (a header, an env variable) that must not be copied
//     into another file. A record an earlier version wrote with Equals is
//     converted to Sum on read, so no legacy value is written back.
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
	// EmptyMaps is the part of Preexisting that held an empty map ({} in YAML), as
	// opposed to a null, so Unmerge puts back what the user wrote.
	EmptyMaps [][]string `json:"emptyMaps,omitempty"`

	// NoFinalNewline records that the document did not end in a newline when
	// ai-rulez first merged into it (Apply adds one), so Unmerge takes the newline
	// it added back out and restores the original bytes.
	NoFinalNewline bool `json:"noFinalNewline,omitempty"`

	// Local records that the machine-local inputs (the overlay, local/ content)
	// shaped what was written, so a run that does not load them leaves it alone.
	Local bool `json:"local,omitempty"`
}

// claimWire is the manifest encoding of a Claim: element digests, never values.
type claimWire struct {
	Path     []string `json:"path"`
	Elements []any    `json:"elements,omitempty"`
	// ElementSums is a pointer so an element claim with no element left is written
	// as "elementSums": [] and read back as an element claim, not as a claim on the
	// whole key (omitempty would drop an empty slice).
	ElementSums    *[]string  `json:"elementSums,omitempty"`
	Equals         any        `json:"equals,omitempty"`
	Sum            string     `json:"sum,omitempty"`
	Alone          bool       `json:"alone,omitempty"`
	Preexisting    [][]string `json:"preexisting,omitempty"`
	EmptyMaps      [][]string `json:"emptyMaps,omitempty"`
	NoFinalNewline bool       `json:"noFinalNewline,omitempty"`
	Local          bool       `json:"local,omitempty"`
}

// MarshalJSON writes the claim with its elements as digests: a manifest records
// which elements are ours without copying their values, which can be secrets.
func (c Claim) MarshalJSON() ([]byte, error) {
	equals, sum := c.persistedGuard()
	var sums *[]string
	if digests := c.ElementDigests(); digests != nil {
		sums = &digests
	}
	return json.Marshal(claimWire{
		Path: c.Path, ElementSums: sums, Equals: equals, Sum: sum, Alone: c.Alone,
		Preexisting: c.Preexisting, EmptyMaps: c.EmptyMaps, NoFinalNewline: c.NoFinalNewline, Local: c.Local,
	})
}

// textClaim reports whether the claim names the one piece of text a document
// keeps verbatim: the generated header above a Markdown block (docmerge's
// "header" key), which unmerge must read back to know what to strip. It is
// ai-rulez's own banner, never a configured value.
func (c Claim) textClaim() bool {
	_, isText := c.Equals.(string)
	return isText && len(c.Path) == 1 && c.Path[0] == textClaimKey
}

const textClaimKey = "header"

// persistedGuard is the guard as a manifest stores it: Equals becomes the digest
// of its value, because a value can hold a secret.
func (c Claim) persistedGuard() (equals any, sum string) {
	if c.Equals == nil || c.textClaim() {
		return c.Equals, c.Sum
	}
	if c.Sum != "" {
		return nil, c.Sum
	}
	return nil, Digest(c.Equals)
}

// UnmarshalJSON reads a claim. A record written before elements were digested
// carries their values; they are folded into digests on the way in so no value
// stays in memory or is written back.
func (c *Claim) UnmarshalJSON(data []byte) error {
	var wire claimWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err //nolint:wrapcheck // decoding error is self-describing
	}
	var sums []string
	if wire.ElementSums != nil {
		sums = *wire.ElementSums
		if sums == nil {
			sums = []string{}
		}
	}
	*c = Claim{
		Path: wire.Path, ElementSums: sums, Equals: wire.Equals, Sum: wire.Sum, Alone: wire.Alone,
		Preexisting: wire.Preexisting, EmptyMaps: wire.EmptyMaps, NoFinalNewline: wire.NoFinalNewline, Local: wire.Local,
	}
	c.Equals, c.Sum = c.persistedGuard()
	if wire.Elements != nil {
		c.ElementSums = c.digestsWith(wire.Elements)
	}
	return nil
}

// HasElements reports whether the claim covers only some elements of an array.
func (c Claim) HasElements() bool { return c.Elements != nil || c.ElementSums != nil }

// ElementDigests lists the digests of the claimed elements, sorted. It is a
// multiset: an element ai-rulez wrote twice is listed twice, so unmerge removes
// only as many identical elements as ai-rulez added and leaves a hand-written
// copy of the same value.
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
	return sums
}

// OwnsElement reports whether value is one of the claimed elements, compared as
// JSON the way a claim read back from a manifest is.
func (c Claim) OwnsElement(value any) bool {
	sum := Digest(value)
	return sum != "" && slices.Contains(c.ElementDigests(), sum)
}

// ElementMatcher hands out the claimed elements one at a time: Take reports
// whether a value is a claimed element that has not been matched yet, so a
// document holding more identical elements than the claim lists keeps the extra
// ones. It is not safe for concurrent use.
type ElementMatcher struct {
	left map[string]int
}

// NewElementMatcher counts the claim's element digests.
func (c Claim) NewElementMatcher() *ElementMatcher {
	m := &ElementMatcher{left: map[string]int{}}
	for _, sum := range c.ElementDigests() {
		m.left[sum]++
	}
	return m
}

// Take consumes one claimed occurrence of value.
func (m *ElementMatcher) Take(value any) bool { return m.takeSum(Digest(value)) }

// TakeRaw is Take for an element still in its raw JSON form.
func (m *ElementMatcher) TakeRaw(raw json.RawMessage) bool { return m.takeSum(digestRaw(raw)) }

func (m *ElementMatcher) takeSum(sum string) bool {
	if sum == "" || m.left[sum] == 0 {
		return false
	}
	m.left[sum]--
	return true
}

// ElementsIn returns the candidates the claim owns, in order, as many copies of
// a value as the claim lists. It is how a caller holding the document's current
// elements learns which of them an earlier run claimed, without the claim
// storing their values.
func (c Claim) ElementsIn(candidates []any) []any {
	var owned []any
	matcher := c.NewElementMatcher()
	for _, candidate := range candidates {
		if matcher.Take(candidate) {
			owned = append(owned, candidate)
		}
	}
	return owned
}

// WithoutElements returns the claim minus the elements other claims.
func (c Claim) WithoutElements(other Claim) Claim {
	drop := other.NewElementMatcher()
	kept := []string{}
	for _, sum := range c.ElementDigests() {
		if !drop.takeSum(sum) {
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
	// UseNumber keeps a number's digits: float64 would give two integers above
	// 2^53 one digest, and the claim would own a value the user had changed.
	var normalized any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if dec.Decode(&normalized) != nil {
		return ""
	}
	if _, err := dec.Token(); err != io.EOF {
		return "" // trailing data after the value
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
			if len(key.Elements) > 0 || key.Created {
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
	// The in-place editor patches only what it removes, so every document comes back
	// byte for byte whatever its layout; it reports a document it cannot parse.
	bom, existing := SplitBOM(existing)
	_, err := decodeObjectMembers([]byte(existing))
	result, err := unmergeJSONC(path, existing, claims, err)
	if result.Changed && !result.Empty {
		result.Body = bom + result.Body
	}
	return result, err
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
