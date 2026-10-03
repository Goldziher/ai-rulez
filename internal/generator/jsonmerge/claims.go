package jsonmerge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/samber/oops"
)

// ErrNotStrictJSON marks a document that is not valid JSON but parses once its
// comments and trailing commas are dropped (JSONC, which Gemini CLI and OpenCode
// accept in their config files). Rewriting it would delete the comments, so
// ai-rulez does not; callers decide whether the keys they own are worth that.
var ErrNotStrictJSON = errors.New("document uses comments or trailing commas")

// Claim records one thing ai-rulez wrote into a merged document, addressed by
// key path. A claim is the only license ai-rulez has to take something out of a
// document the consumer owns.
//
//   - Path alone: the key at Path (an MCP server entry, a scalar) is ours.
//   - Elements: the key at Path is the consumer's array and only these elements
//     of it are ours.
//   - Equals, when set, restricts removal to a value that still equals it.
//   - Alone restricts removal to when no other top-level key remains.
type Claim struct {
	Path     []string `json:"path"`
	Elements []any    `json:"elements,omitempty"`
	Equals   any      `json:"equals,omitempty"`
	Alone    bool     `json:"alone,omitempty"`
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
			claims = append(claims, Claim{Path: segs})
		}
	}
	return claims
}

// memberClaims names every entry of a map value, sorted so the record is
// deterministic. A map with no entries claims the key itself.
func memberClaims(segs []string, value any) []Claim {
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String || rv.Len() == 0 {
		return []Claim{{Path: segs}}
	}
	names := make([]string, 0, rv.Len())
	for _, k := range rv.MapKeys() {
		names = append(names, k.String())
	}
	sort.Strings(names)
	claims := make([]Claim, 0, len(names))
	for _, name := range names {
		claims = append(claims, Claim{Path: append(append([]string{}, segs...), name)})
	}
	return claims
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
	members, err := decodeObjectMembers([]byte(existing))
	if err != nil {
		return Unmerged{}, parseFailure(path, existing, err)
	}

	indent := detectTopLevelIndent(existing)
	newline := detectLineEnding(existing)
	members, changed, err := unmergeAll(members, claims, indent, newline)
	if err != nil {
		return Unmerged{}, oops.With("path", path).Wrapf(err, "remove ai-rulez content from JSON settings document")
	}
	if !changed {
		return Unmerged{}, nil
	}
	rendered, err := renderMembers(members, 1, indent, newline)
	if err != nil {
		return Unmerged{}, oops.With("path", path).Wrapf(err, "encode JSON settings document")
	}
	return Unmerged{Body: rendered + newline, Changed: true, Empty: len(members) == 0}, nil
}

// unmergeAll applies the claims in order, the Alone ones last and only while a
// single top-level member remains.
func unmergeAll(members []jsonMember, claims []Claim, indent, newline string) ([]jsonMember, bool, error) {
	changed := false
	for _, alone := range []bool{false, true} {
		for _, claim := range claims {
			if len(claim.Path) == 0 || claim.Alone != alone || (alone && len(members) != 1) {
				continue
			}
			next, did, err := unmergeClaim(members, claim.Path, claim, 1, indent, newline)
			if err != nil {
				return nil, false, err
			}
			members, changed = next, changed || did
		}
	}
	return members, changed, nil
}

// unmergeClaim removes the member (or array elements) the claim addresses,
// dropping an ancestor object the removal leaves empty.
func unmergeClaim(members []jsonMember, path []string, claim Claim, depth int, indent, newline string,
) ([]jsonMember, bool, error) {
	head, rest := path[0], path[1:]
	idx := -1
	for i, member := range members {
		if member.Key == head {
			idx = i
			break
		}
	}
	if idx < 0 {
		return members, false, nil
	}

	if len(rest) == 0 {
		return unmergeLeaf(members, idx, claim, depth, indent, newline)
	}
	raw := bytes.TrimSpace(members[idx].Raw)
	if len(raw) == 0 || raw[0] != '{' {
		return members, false, nil
	}
	child, err := decodeObjectMembers(raw)
	if err != nil {
		return nil, false, err
	}
	child, did, err := unmergeClaim(child, rest, claim, depth+1, indent, newline)
	if err != nil || !did {
		return members, false, err
	}
	if len(child) == 0 {
		return removeMember(members, head), true, nil
	}
	rendered, err := renderMembers(child, depth+1, indent, newline)
	if err != nil {
		return nil, false, err
	}
	members[idx] = jsonMember{Key: head, Raw: []byte(rendered)}
	return members, true, nil
}

// unmergeLeaf applies a claim to the member at members[idx].
func unmergeLeaf(members []jsonMember, idx int, claim Claim, depth int, indent, newline string,
) ([]jsonMember, bool, error) {
	head := members[idx].Key
	if claim.Equals != nil && !jsonEquals(members[idx].Raw, claim.Equals) {
		return members, false, nil
	}
	if claim.Elements == nil {
		return removeMember(members, head), true, nil
	}

	var elements []json.RawMessage
	if err := json.Unmarshal(members[idx].Raw, &elements); err != nil {
		return members, false, nil //nolint:nilerr // not an array: the consumer's value, left alone
	}
	kept := make([]json.RawMessage, 0, len(elements))
	for _, element := range elements {
		if !anyEquals(element, claim.Elements) {
			kept = append(kept, element)
		}
	}
	if len(kept) == len(elements) {
		return members, false, nil
	}
	if len(kept) == 0 {
		return removeMember(members, head), true, nil
	}
	rendered, err := replaceMemberValue(members, head, kept, depth, indent, newline)
	return rendered, true, err
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

// parseFailure builds the error for a document decodeObjectMembers rejected,
// marking the JSONC case (see ErrNotStrictJSON).
func parseFailure(path, doc string, cause error) error {
	if stripped := stripJSONC(doc); stripped != doc {
		if _, err := decodeObjectMembers([]byte(stripped)); err == nil {
			return oops.
				With("path", path).
				Hint(fmt.Sprintf("%s has comments or trailing commas; ai-rulez cannot merge into it without deleting them.", path)).
				Wrapf(ErrNotStrictJSON, "parse existing JSON settings document")
		}
	}
	return oops.
		With("path", path).
		Hint(fmt.Sprintf(
			"%s is not parseable JSON, and ai-rulez will not overwrite a file it cannot merge into. "+
				"Fix the syntax (comments and trailing commas are not valid JSON), or move the file aside.", path)).
		Wrapf(cause, "parse existing JSON settings document")
}

// stripJSONC drops // and /* */ comments and trailing commas that sit outside
// string literals. Anything it leaves behind that is still not JSON stays an
// error for the caller.
func stripJSONC(doc string) string {
	var b []byte
	inString, escaped := false, false
	for i := 0; i < len(doc); i++ {
		c := doc[i]
		if inString {
			inString, escaped = advanceWithinString(c, escaped)
			b = append(b, c)
			continue
		}
		switch {
		case c == '"':
			inString = true
			b = append(b, c)
		case c == '/' && i+1 < len(doc) && doc[i+1] == '/':
			for i < len(doc) && doc[i] != '\n' {
				i++
			}
			if i < len(doc) {
				b = append(b, '\n')
			}
		case c == '/' && i+1 < len(doc) && doc[i+1] == '*':
			end := bytes.Index([]byte(doc[i+2:]), []byte("*/"))
			if end < 0 {
				return doc
			}
			i += end + 3
			b = append(b, ' ')
		default:
			b = append(b, c)
		}
	}
	return dropTrailingCommas(string(b))
}

// dropTrailingCommas removes a comma whose next significant character closes an
// object or array.
func dropTrailingCommas(doc string) string {
	var b []byte
	inString, escaped := false, false
	for i := 0; i < len(doc); i++ {
		c := doc[i]
		if inString {
			inString, escaped = advanceWithinString(c, escaped)
			b = append(b, c)
			continue
		}
		if c == '"' {
			inString = true
		}
		if c == ',' {
			j := i + 1
			for j < len(doc) && (doc[j] == ' ' || doc[j] == '\t' || doc[j] == '\n' || doc[j] == '\r') {
				j++
			}
			if j < len(doc) && (doc[j] == '}' || doc[j] == ']') {
				continue
			}
		}
		b = append(b, c)
	}
	return string(b)
}
