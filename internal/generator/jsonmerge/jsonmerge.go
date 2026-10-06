// Package jsonmerge rewrites the top-level keys ai-rulez owns inside a
// settings-style JSON document without disturbing anything else in it.
//
// Documents such as .claude/settings.json, .gemini/settings.json and /.mcp.json
// are shared: ai-rulez owns one or two top-level keys (typically mcpServers) and
// the consumer hand-authors and version-controls the rest — permissions, hooks,
// skillOverrides, editor settings. Rendering them from a fresh Go map destroyed
// everything ai-rulez does not own (#185), so every generator that emits an
// object-shaped JSON document goes through Apply.
//
// A document with comments or trailing commas (JSONC, as VS Code, Zed and
// OpenCode write them) is edited in place instead, so its comments survive; see
// jsonc.go. Output for strict JSON is unchanged by that.
//
// The package is a leaf on purpose: both internal/generator/presets and
// internal/generator/providers depend on it, and providers already depends on
// presets, so the merge machinery cannot live in either of them.
package jsonmerge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"reflect"
	"strings"

	"github.com/samber/oops"
)

// defaultJSONIndent is the indentation used when ai-rulez creates a JSON
// document from scratch, and the fallback when an existing document's own
// indentation cannot be detected (e.g. it is a single line).
const defaultJSONIndent = "  "

// errNotJSONObject marks an existing document whose root is not a JSON object.
// Merging owned keys into an array or scalar is undefined, so generation stops
// rather than replacing a file whose shape we do not understand.
var errNotJSONObject = errors.New("root value is not a JSON object")

// errTrailingJSONContent marks an existing document with content after its root
// object. Rewriting the object would drop that content, so generation stops.
var errTrailingJSONContent = errors.New("unexpected content after the root JSON object")

// OwnedKey is a single key ai-rulez owns inside an otherwise user-authored JSON
// document, paired with the value to write. Name addresses a top-level key;
// Path, when set, addresses a nested key such as {"mcp":{"servers":{...}}}, so
// sibling keys the document already carries under the same ancestor survive.
//
// A slice of keys rather than a map so that merging into an existing document
// appends new keys in a deterministic sequence. Creating a document from scratch
// goes through marshalOwnedJSONKeys instead, which sorts by key to stay
// byte-identical to pre-merge output.
//
// Remove deletes the addressed key instead of writing Value (and drops an
// ancestor object the removal leaves empty), for a key ai-rulez wrote earlier
// and no longer owns. Removing an absent key is a no-op.
type OwnedKey struct {
	Name   string
	Value  any
	Path   []string
	Remove bool

	// RemoveIf, when set on a Remove key, guards the removal: the key is deleted
	// only when RemoveIf accepts its current raw JSON value, so a value the
	// consumer rewrote or authored is left alone.
	RemoveIf func(raw json.RawMessage) bool

	// Members marks Value as a map whose entries are each ai-rulez's (the MCP
	// server map). Apply then merges the entries one by one into the map the
	// document already has, so an entry the consumer wrote under another name
	// survives; an entry of the same name is replaced (ai-rulez's wins). The
	// ownership record names every entry instead of the whole key, each guarded
	// by the value written. An entry that dropped out of Value is not removed
	// here; the previous record's claim for it is (see Unmerge).
	Members bool

	// Elements marks Value as an array of which ai-rulez added only these
	// elements; the rest are the consumer's. A non-nil empty slice claims
	// nothing. See Claim.
	Elements []any

	// Alone marks a scalar ai-rulez added only so that the rest of the document is
	// valid (Cursor's hooks.json `version`): clean takes it back only when no other
	// top-level key remains.
	Alone bool
}

// segments returns the key path this OwnedKey addresses. Path wins when set;
// otherwise the single Name is the path.
func (k OwnedKey) segments() []string {
	if len(k.Path) > 0 {
		return k.Path
	}
	if k.Name == "" {
		return nil
	}
	return []string{k.Name}
}

// Result is the outcome of one merge: the body to write, plus whether the
// document turned out to be shared with the consumer.
//
// PartiallyOwned is derived from what was actually on disk rather than from the
// kind of document, and that distinction matters. A .mcp.json that ai-rulez
// created itself holds nothing but the owned mcpServers key, so it is wholly
// generated and the pipeline may gitignore it — which is what keeps resolved MCP
// secrets out of git. The same path in a repo that hand-authored it carries the
// consumer's own keys, and ignoring or deleting that file would hide or destroy
// their work (#185). Keying on content makes the answer stable across runs: it
// depends on which keys the document has, not on whether a previous run happened
// to create it.
type Result struct {
	Body           string
	PartiallyOwned bool
	// Claims record what ai-rulez wrote, so that Unmerge can take exactly that
	// back out when the owning preset or server goes away or on clean.
	Claims []Claim
	// Owned are the keys the document was merged from. docmerge.Apply sets it, so
	// a caller that must merge another preset's keys into the same document (see
	// config.MergeSource) can render the union.
	Owned []OwnedKey
}

// jsonMember is one top-level key/value pair of a JSON object, with the value
// kept as the exact source bytes. Preserving raw bytes is what lets untouched
// keys round-trip unchanged — including nested indentation and number
// formatting that a map[string]any round-trip would normalize away.
type jsonMember struct {
	Key string
	Raw json.RawMessage
}

// Apply renders an object-shaped JSON document by replacing only the keys
// ai-rulez owns in the document that already exists at path, leaving every other
// member byte-for-byte and in its original position. When path is empty or no
// file is there yet, the document is created from the owned keys alone.
//
// The returned Result reports whether the merged document still holds any member
// ai-rulez does not own, which is what makes the file the consumer's rather than
// a generated artifact.
func Apply(path string, owned []OwnedKey) (Result, error) {
	existing, found, err := readExistingDocument(path)
	if err != nil {
		return Result{}, err
	}
	if !found {
		body, err := marshalOwnedJSONKeys(owned)
		return Result{Body: body, Claims: claimsFor(owned)}, err
	}
	return ApplyDocument(path, existing, owned)
}

// hasCompactContainer reports whether a one-line document has members, or a
// multi-line one has a non-empty object or array, below the top level, written
// on one line. The strict engine
// re-renders any container it edits in its own multi-line layout, so such a
// container would come back re-indented and clean could not restore the original
// bytes; the in-place editor (jsonc.go) patches only what it changes, so those
// documents go there.
func hasCompactContainer(doc string, members []jsonMember) bool {
	if len(members) > 0 && !strings.Contains(strings.TrimRight(doc, "\r\n"), "\n") {
		return true // a one-line document: the editor keeps its single line
	}
	var open []int // offset of each container still open
	inString, escaped := false, false
	for i := 0; i < len(doc); i++ {
		c := doc[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{', '[':
			open = append(open, i)
		case '}', ']':
			if len(open) == 0 {
				return false
			}
			start := open[len(open)-1]
			open = open[:len(open)-1]
			inner := doc[start+1 : i]
			if len(open) > 0 && strings.TrimSpace(inner) != "" && !strings.ContainsAny(inner, "\r\n") {
				return true
			}
		}
	}
	return false
}

// ApplyDocument is Apply for a document already read, which must not be empty;
// path only names it in errors.
func ApplyDocument(path, existing string, owned []OwnedKey) (Result, error) {
	bom, existing := SplitBOM(existing)
	members, err := decodeObjectMembers([]byte(existing))
	if err != nil || hasCompactContainer(existing, members) {
		// Not strict JSON: it may be JSONC, which is edited in place so its
		// comments survive (see jsonc.go).
		result, err := applyJSONC(path, existing, owned, err)
		if err == nil {
			result.Body = bom + result.Body
		}
		return result, err
	}

	indent := detectTopLevelIndent(existing)
	newline := detectLineEnding(existing)
	merged := members
	for _, key := range owned {
		segs := key.segments()
		if len(segs) == 0 {
			continue
		}
		merged, err = replaceOwnedPath(merged, segs, key, 1, indent, newline)
		if err != nil {
			return Result{}, oops.With("path", path).Wrapf(err, "merge owned keys into JSON settings document")
		}
	}
	rendered, err := renderMembers(merged, 1, indent, newline)
	if err != nil {
		return Result{}, oops.With("path", path).Wrapf(err, "encode merged JSON settings document")
	}
	body := rendered + newline
	if err := checkPreservedDocuments(existing, body, func(before, after map[string]any) error {
		return CheckPreservedApply(before, after, owned)
	}); err != nil {
		return Result{}, oops.With("path", path).
			Hint("ai-rulez could not merge its keys into this JSON document without altering the rest of it; the file was left untouched").
			Wrapf(err, "merged JSON settings document does not preserve the existing content")
	}
	partial := hasUnownedMembers(merged, owned)
	if tree, err := DecodeTree(body); err == nil && HasUserElements(tree, owned) {
		partial = true
	}
	return Result{
		Body:           bom + body,
		PartiallyOwned: partial,
		Claims:         NoteFinalNewline(claimsFor(owned), existing),
	}, nil
}

// checkPreservedDocuments decodes both strict JSON documents and runs check on
// the results.
func checkPreservedDocuments(before, after string, check func(before, after map[string]any) error) error {
	b, err := DecodeTree(before)
	if err != nil {
		return err
	}
	a, err := DecodeTree(after)
	if err != nil {
		return err
	}
	return check(b, a)
}

// hasUnownedMembers reports whether the document carries a top-level key outside
// the owned set. Such a key can only have come from the consumer, so the file is
// theirs to track and ai-rulez must not ignore or delete it.
func hasUnownedMembers(members []jsonMember, owned []OwnedKey) bool {
	for _, key := range owned {
		if key.Elements != nil && sliceLen(key.Value) > len(key.Elements) {
			// The written array also holds elements ai-rulez does not own.
			return true
		}
	}
	return hasUnownedPaths(members, ownedPaths(owned))
}

// ownedPaths lists the key path of every owned key, expanding a Members key into
// one path per entry written.
func ownedPaths(owned []OwnedKey) [][]string {
	paths := make([][]string, 0, len(owned))
	for _, key := range owned {
		segs := key.segments()
		if len(segs) == 0 {
			continue
		}
		if key.Members {
			// Only the entries written are ours; an empty map owns none of what the
			// document already holds there.
			names, _, _ := memberEntries(key.Value)
			for _, name := range names {
				paths = append(paths, append(append([]string{}, segs...), name))
			}
			continue
		}
		paths = append(paths, segs)
	}
	return paths
}

// sliceLen is the length of a slice or array value, and 0 for anything else.
func sliceLen(value any) int {
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		return rv.Len()
	}
	return 0
}

// hasUnownedPaths reports whether any member is outside the owned paths. A member
// addressed by a whole path is owned entirely; one addressed only by deeper paths
// is inspected recursively, so a sibling the document carries under an owned
// ancestor (mcp.timeout beside the owned mcp.servers) still counts as the
// consumer's.
func hasUnownedPaths(members []jsonMember, paths [][]string) bool {
	roots := make(map[string][][]string, len(paths))
	for _, path := range paths {
		roots[path[0]] = append(roots[path[0]], path[1:])
	}

	for _, member := range members {
		rems, ok := roots[member.Key]
		if !ok {
			return true
		}

		var nested [][]string
		wholeOwned := false
		for _, rem := range rems {
			if len(rem) == 0 {
				wholeOwned = true
				break
			}
			nested = append(nested, rem)
		}
		if wholeOwned {
			continue
		}

		childMembers, err := decodeObjectMembers(bytes.TrimSpace(member.Raw))
		if err != nil {
			// A non-object we cannot introspect must be treated as the
			// consumer's rather than silently ignored.
			return true
		}
		if hasUnownedPaths(childMembers, nested) {
			return true
		}
	}
	return false
}

// readExistingDocument reads the current contents of the target document. A
// missing file and an all-whitespace file are both reported as "not found" so
// generation creates a fresh document instead of failing to parse nothing. Any
// other read failure stops generation: silently treating an unreadable file as
// absent is how the clobbering bug behaved.
func readExistingDocument(path string) (contents string, found bool, err error) {
	if path == "" {
		return "", false, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from the provider spec + config base dir
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, nil
		}
		return "", false, oops.
			With("path", path).
			Hint(fmt.Sprintf("Check read permissions for: %s", path)).
			Wrapf(err, "read existing JSON settings document")
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", false, nil
	}
	return string(data), true, nil
}

// marshalOwnedJSONKeys renders a brand-new document containing only the owned
// keys. Kept on the plain map + MarshalIndent path so greenfield output is
// byte-identical to what ai-rulez emitted before merging existed.
func marshalOwnedJSONKeys(owned []OwnedKey) (string, error) {
	payload := make(map[string]any, len(owned))
	for _, key := range owned {
		segs := key.segments()
		if len(segs) == 0 || key.Remove {
			continue
		}
		insertOwnedValue(payload, segs, key.Value)
	}
	jsonBytes, err := json.MarshalIndent(payload, "", defaultJSONIndent)
	if err != nil {
		return "", oops.Wrapf(err, "marshal JSON settings document")
	}
	return string(jsonBytes) + "\n", nil
}

// insertOwnedValue places value at the nested path inside node, creating
// intermediate objects. The last write wins for a path re-declared by several
// owned keys, which no caller does.
func insertOwnedValue(node map[string]any, path []string, value any) {
	for i := 0; i < len(path)-1; i++ {
		child, ok := node[path[i]].(map[string]any)
		if !ok {
			child = make(map[string]any)
			node[path[i]] = child
		}
		node = child
	}
	node[path[len(path)-1]] = value
}

// decodeObjectMembers streams the members of a JSON object, capturing each value
// as its original source bytes. Comments and trailing commas make this fail;
// Apply and Unmerge then fall back to the comment-preserving JSONC path.
func decodeObjectMembers(data []byte) ([]jsonMember, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	open, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := open.(json.Delim); !ok || delim != '{' {
		return nil, errNotJSONObject
	}

	var members []jsonMember
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, errNotJSONObject
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, err
		}
		members = append(members, jsonMember{Key: key, Raw: raw})
	}
	// Consume the closing brace, then require end-of-input: anything after the
	// root object would be dropped by the rewrite, so refuse instead.
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errTrailingJSONContent
	}
	return members, nil
}

// replaceOwnedPath rewrites the member addressed by path in place, descending
// into nested objects so only the addressed key changes, and appends the member
// (and any missing ancestors) when the document did not have it. Duplicate
// occurrences of an owned key (legal but ambiguous JSON) collapse into the first
// position.
func replaceOwnedPath(members []jsonMember, path []string, key OwnedKey, depth int, indent, newline string,
) ([]jsonMember, error) {
	head, rest := path[0], path[1:]

	if len(rest) == 0 {
		if key.Remove {
			if key.RemoveIf != nil && !removalAccepted(members, head, key.RemoveIf) {
				return members, nil
			}
			return removeMember(members, head), nil
		}
		if key.Members {
			return mergeMembers(members, head, key.Value, depth, indent, newline)
		}
		return replaceMemberValue(members, head, key.Value, depth, indent, newline)
	}

	idx := -1
	for i, member := range members {
		if member.Key == head {
			idx = i
			break
		}
	}

	childMembers, err := childObjectMembers(members, idx, head)
	if err != nil {
		return nil, err
	}
	if key.Remove && idx < 0 {
		return members, nil
	}
	child, err := replaceOwnedPath(childMembers, rest, key, depth+1, indent, newline)
	if err != nil {
		return nil, err
	}
	if key.Remove && len(child) == 0 {
		return removeMember(members, head), nil
	}
	raw, err := renderMembers(child, depth+1, indent, newline)
	if err != nil {
		return nil, err
	}

	if idx >= 0 {
		members[idx] = jsonMember{Key: head, Raw: []byte(raw)}
		return members, nil
	}
	return append(members, jsonMember{Key: head, Raw: []byte(raw)}), nil
}

// removalAccepted reports whether the member head exists and accept takes its value.
func removalAccepted(members []jsonMember, head string, accept func(json.RawMessage) bool) bool {
	for _, member := range members {
		if member.Key == head {
			return accept(json.RawMessage(member.Raw))
		}
	}
	return false
}

// mergeMembers writes each entry of value into the object at head, replacing the
// entry of the same name in place and appending the others in name order, and
// leaves every other entry of that object as it is. An absent object is created.
// A value with no entries leaves an existing object alone.
func mergeMembers(members []jsonMember, head string, value any, depth int, indent, newline string,
) ([]jsonMember, error) {
	names, entries, ok := memberEntries(value)
	idx := -1
	for i, member := range members {
		if member.Key == head {
			idx = i
			break
		}
	}
	if !ok {
		if idx >= 0 {
			return members, nil
		}
		return replaceMemberValue(members, head, value, depth, indent, newline)
	}

	child, err := childObjectMembers(members, idx, head)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		child, err = replaceMemberValue(child, name, entries[name], depth+1, indent, newline)
		if err != nil {
			return nil, err
		}
	}
	raw, err := renderMembers(child, depth+1, indent, newline)
	if err != nil {
		return nil, err
	}
	if idx >= 0 {
		members[idx] = jsonMember{Key: head, Raw: []byte(raw)}
		return members, nil
	}
	return append(members, jsonMember{Key: head, Raw: []byte(raw)}), nil
}

// childObjectMembers returns the members of the object at members[idx], or nil
// when idx is absent (the caller creates the object). A present value that is not
// an object is refused: merging into it would have to replace the consumer's
// value outright, which is the clobbering this package exists to prevent.
func childObjectMembers(members []jsonMember, idx int, head string) ([]jsonMember, error) {
	if idx < 0 {
		return nil, nil
	}
	raw := bytes.TrimSpace(members[idx].Raw)
	if len(raw) == 0 || raw[0] != '{' {
		return nil, oops.
			With("key", head).
			Hint("ai-rulez merges its keys into an object and will not replace a non-object value it cannot preserve").
			Errorf("existing key %q is not a JSON object", head)
	}
	return decodeObjectMembers(raw)
}

// removeMember drops every member named name.
func removeMember(members []jsonMember, name string) []jsonMember {
	kept := make([]jsonMember, 0, len(members))
	for _, member := range members {
		if member.Key != name {
			kept = append(kept, member)
		}
	}
	return kept
}

// replaceMemberValue replaces the single member named name, or appends it. The
// value is indented as a member at the given depth: MarshalIndent's prefix is
// applied to every line after the first, which is exactly the nesting a value at
// that depth needs.
func replaceMemberValue(members []jsonMember, name string, value any, depth int, indent, newline string) ([]jsonMember, error) {
	valueBytes, err := json.MarshalIndent(value, strings.Repeat(indent, depth), indent)
	if err != nil {
		return nil, fmt.Errorf("marshal owned key %q: %w", name, err)
	}
	// MarshalIndent always breaks lines with LF, so the rendered value would be
	// the one LF island in a CRLF document.
	if newline != "\n" {
		valueBytes = bytes.ReplaceAll(valueBytes, []byte("\n"), []byte(newline))
	}

	replaced := false
	kept := make([]jsonMember, 0, len(members)+1)
	for _, member := range members {
		if member.Key != name {
			kept = append(kept, member)
			continue
		}
		if replaced {
			continue
		}
		kept = append(kept, jsonMember{Key: name, Raw: valueBytes})
		replaced = true
	}
	if !replaced {
		kept = append(kept, jsonMember{Key: name, Raw: valueBytes})
	}
	return kept, nil
}

// renderMembers writes members as an indented JSON object, one member per line,
// with each value emitted verbatim. No trailing newline is added, so the result
// is usable both as a whole document and as a nested member value; the caller
// appends the document-level newline.
func renderMembers(members []jsonMember, depth int, indent, newline string) (string, error) {
	if len(members) == 0 {
		return "{}", nil
	}

	pad := strings.Repeat(indent, depth)
	var b strings.Builder
	b.WriteString("{" + newline)
	for i, member := range members {
		b.WriteString(pad)
		// Re-quote through encoding/json so key escaping matches the rest of the
		// document rather than Go's strconv rules.
		keyBytes, err := json.Marshal(member.Key)
		if err != nil {
			return "", fmt.Errorf("marshal key %q: %w", member.Key, err)
		}
		b.Write(keyBytes)
		b.WriteString(": ")
		b.Write(member.Raw)
		if i < len(members)-1 {
			b.WriteString(",")
		}
		b.WriteString(newline)
	}
	b.WriteString(strings.Repeat(indent, depth-1) + "}")
	return b.String(), nil
}

// detectLineEnding reports the line ending the document already uses. An
// untouched member is re-emitted byte for byte, so its internal CRLFs survive
// regardless; writing LF around them left a file carrying both, which reads as a
// whole-file change to git and to the editor that wrote it.
func detectLineEnding(doc string) string {
	if strings.Contains(doc, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// detectTopLevelIndent infers the indentation of an existing document from the
// first line that starts a top-level member, so a file indented with four spaces
// or tabs is not reformatted to two. Falls back to defaultJSONIndent for a
// single-line document, or one whose members all sit on the opening line.
//
// The member has to be located rather than guessed at. Reading the line right
// after the opening brace measures zero on a document with a blank line there.
// Taking the first indented line instead reads nested indentation whenever the
// first key opens an object or array on the brace line --
//
//	{"permissions": {
//	    "allow": []
//	  },
//	  "model": "opus"
//	}
//
// where the first indented line is "allow" at four spaces, not the two the
// document actually uses. Every hand-authored member then gets rewritten at the
// wrong width, which is the whole-file diff this package exists to avoid. So
// track brace depth (ignoring braces inside strings) and take the first line
// whose leading non-blank character opens a key at depth one.
func detectTopLevelIndent(doc string) string {
	depth, lineStart := 0, 0
	inString, escaped, seenContent := false, false, false

	for i := 0; i < len(doc); i++ {
		char := doc[i]

		if inString {
			inString, escaped = advanceWithinString(char, escaped)
			continue
		}
		if char == '\n' {
			lineStart, seenContent = i+1, false
			continue
		}
		if char == ' ' || char == '\t' || char == '\r' {
			continue
		}

		// First non-blank character on this line. A top-level member always
		// starts with the quote of its key and sits at depth 1, directly inside
		// the root object; anything deeper belongs to a nested value, and the
		// root's own closing brace is not a member.
		if !seenContent {
			seenContent = true
			if depth == 1 && char == '"' && i > lineStart {
				return doc[lineStart:i]
			}
		}

		inString, depth = advanceStructural(char, depth)
	}

	return defaultJSONIndent
}

// advanceWithinString steps one byte of a string literal, reporting whether the
// literal continues and whether the next byte is escaped.
func advanceWithinString(char byte, escaped bool) (stillInString, nowEscaped bool) {
	switch {
	case escaped:
		return true, false
	case char == '\\':
		return true, true
	case char == '"':
		return false, false
	default:
		return true, false
	}
}

// advanceStructural steps one byte outside a string literal, reporting whether a
// string just opened and the resulting nesting depth.
func advanceStructural(char byte, depth int) (inString bool, newDepth int) {
	switch char {
	case '"':
		return true, depth
	case '{', '[':
		return false, depth + 1
	case '}', ']':
		return false, depth - 1
	default:
		return false, depth
	}
}
