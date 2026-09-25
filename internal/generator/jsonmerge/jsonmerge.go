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

// OwnedKey is a single top-level key ai-rulez owns inside an otherwise
// user-authored JSON document, paired with the value to write.
//
// A slice rather than a map so that merging into an existing document appends new
// keys in a deterministic sequence. Creating a document from scratch goes through
// marshalOwnedJSONKeys instead, which sorts by key to stay byte-identical to
// pre-merge output; the two agree for any caller passing a single key, which is
// every caller today.
type OwnedKey struct {
	Name  string
	Value any
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
		return Result{Body: body}, err
	}

	members, err := decodeTopLevelMembers([]byte(existing))
	if err != nil {
		return Result{}, oops.
			With("path", path).
			Hint(fmt.Sprintf(
				"%s is not parseable JSON, and ai-rulez will not overwrite a file it cannot merge into. "+
					"Fix the syntax (comments and trailing commas are not valid JSON), or move the file aside.", path)).
			Wrapf(err, "parse existing JSON settings document")
	}

	indent := detectTopLevelIndent(existing)
	merged, err := replaceOwnedMembers(members, owned, indent)
	if err != nil {
		return Result{}, oops.With("path", path).Wrapf(err, "merge owned keys into JSON settings document")
	}
	encoded, err := encodeTopLevelMembers(merged, indent)
	if err != nil {
		return Result{}, oops.With("path", path).Wrapf(err, "encode merged JSON settings document")
	}
	return Result{Body: encoded, PartiallyOwned: hasUnownedMembers(merged, owned)}, nil
}

// hasUnownedMembers reports whether the document carries a top-level key outside
// the owned set. Such a key can only have come from the consumer, so the file is
// theirs to track and ai-rulez must not ignore or delete it.
func hasUnownedMembers(members []jsonMember, owned []OwnedKey) bool {
	ownedNames := make(map[string]bool, len(owned))
	for _, key := range owned {
		ownedNames[key.Name] = true
	}
	for _, member := range members {
		if !ownedNames[member.Key] {
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
		payload[key.Name] = key.Value
	}
	jsonBytes, err := json.MarshalIndent(payload, "", defaultJSONIndent)
	if err != nil {
		return "", oops.Wrapf(err, "marshal JSON settings document")
	}
	return string(jsonBytes) + "\n", nil
}

// decodeTopLevelMembers streams the top-level members of a JSON object,
// capturing each value as its original source bytes. Comments, trailing commas
// and any other JSON5/JSONC extension make this fail — deliberately, since
// encoding/json cannot round-trip them.
func decodeTopLevelMembers(data []byte) ([]jsonMember, error) {
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

// replaceOwnedMembers rewrites the owned members in place and appends the ones
// the document did not have yet. Duplicate occurrences of an owned key (legal
// but ambiguous JSON) collapse into the first position.
func replaceOwnedMembers(members []jsonMember, owned []OwnedKey, indent string) ([]jsonMember, error) {
	for _, key := range owned {
		// Indent the owned value as a member of the root object: MarshalIndent's
		// prefix is applied to every line after the first, which is exactly the
		// nesting a depth-1 value needs.
		valueBytes, err := json.MarshalIndent(key.Value, indent, indent)
		if err != nil {
			return nil, fmt.Errorf("marshal owned key %q: %w", key.Name, err)
		}

		replaced := false
		kept := make([]jsonMember, 0, len(members)+1)
		for _, member := range members {
			if member.Key != key.Name {
				kept = append(kept, member)
				continue
			}
			if replaced {
				continue
			}
			kept = append(kept, jsonMember{Key: key.Name, Raw: valueBytes})
			replaced = true
		}
		if !replaced {
			kept = append(kept, jsonMember{Key: key.Name, Raw: valueBytes})
		}
		members = kept
	}
	return members, nil
}

// encodeTopLevelMembers writes members back as an indented JSON object, one
// member per line, with each value emitted verbatim.
func encodeTopLevelMembers(members []jsonMember, indent string) (string, error) {
	if len(members) == 0 {
		return "{}\n", nil
	}
	var b strings.Builder
	b.WriteString("{\n")
	for i, member := range members {
		b.WriteString(indent)
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
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.String(), nil
}

// detectTopLevelIndent infers the indentation of an existing document from its
// first indented line, so a file indented with four spaces or tabs is not
// reformatted to two. Falls back to defaultJSONIndent for a single-line document
// or one whose top-level members are all on the opening line.
//
// Scanning for the first indented line rather than reading the line right after
// the opening brace matters: a document with a blank line after '{', or with
// leading blank lines before it, would otherwise measure a width of zero and get
// its whole top level silently re-indented.
func detectTopLevelIndent(doc string) string {
	for _, line := range strings.Split(doc, "\n") {
		width := 0
		for width < len(line) && (line[width] == ' ' || line[width] == '\t') {
			width++
		}
		// A line that is only whitespace says nothing about member indentation.
		if width == 0 || width == len(line) {
			continue
		}
		return line[:width]
	}
	return defaultJSONIndent
}
