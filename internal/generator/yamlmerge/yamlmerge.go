// Package yamlmerge rewrites the keys ai-rulez owns inside a user-authored YAML
// document (.poolside/settings.yaml, .rovodev/config.yml, .takt/config.yaml,
// ...) without disturbing anything else in it.
//
// It follows the contract of jsonmerge and shares its types: OwnedKey, Result,
// Claim and Unmerged. A key path addresses nested mappings, and Members merges a
// map entry by entry, which is how an MCP server map is written without
// touching servers the user added.
//
// yaml.v3 locates each key (line and column) and renders the owned values, but
// the document is edited as text: only the owned members are replaced, inserted
// or removed, so comments, blank lines, indentation and key order of everything
// else survive byte for byte. Re-encoding the whole node tree would not (it
// drops blank lines and renormalises indentation). A flow-style mapping
// ({a: 1}) has no line structure to splice, so the member holding one is
// re-rendered from its nodes, which keeps the comments yaml.v3 attaches to them.
//
// Every result is parsed again, each owned value is checked and everything that
// is not owned is compared with the original, so an edit that went wrong is an
// error rather than a corrupted file. Only block-style root mappings of a single
// document are supported; anything else is refused, as is a document that uses
// anchors, aliases or merge keys, or a line break other than LF and CRLF (a lone
// CR, NEL, LS, PS), because the text edits would no longer line up with what the
// parser reports. A block scalar that runs to the end of a file with no final
// newline cannot take a member after it without gaining a newline of its own, so
// that edit is refused too.
package yamlmerge

import (
	"errors"
	"io"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// OwnedKey is a key ai-rulez owns, shared with jsonmerge. See jsonmerge.OwnedKey.
type OwnedKey = jsonmerge.OwnedKey

// Result is the outcome of one merge, shared with jsonmerge.
type Result = jsonmerge.Result

// Claim records what ai-rulez wrote, shared with jsonmerge.
type Claim = jsonmerge.Claim

// Unmerged is the outcome of Unmerge, shared with jsonmerge.
type Unmerged = jsonmerge.Unmerged

const defaultIndent = 2

// Apply merges the owned keys into the YAML document at path, leaving every
// other member as it was. When path is empty or no file is there yet, the
// document is created from the owned keys alone.
func Apply(path string, owned []OwnedKey) (Result, error) {
	existing, found, err := jsonmerge.ReadExisting(path)
	if err != nil {
		return Result{}, err
	}
	if !found {
		body, err := renderOwned(owned)
		return Result{Body: body, Claims: jsonmerge.ClaimsFor(owned)}, err
	}
	return ApplyDocument(path, existing, owned)
}

// ApplyDocument is Apply for a document already read; path only names it in
// errors.
func ApplyDocument(path, existing string, owned []OwnedKey) (Result, error) {
	bom, existing := jsonmerge.SplitBOM(existing)
	before, err := parse(existing)
	if err != nil {
		return Result{}, parseFailure(path, err)
	}
	ed := newEditor(existing)
	for _, key := range owned {
		segs := key.Segments()
		if len(segs) == 0 {
			continue
		}
		if err := ed.apply(key, segs); err != nil {
			return Result{}, oops.With("path", path).Wrapf(err, "merge owned keys into YAML document")
		}
	}

	parsed, err := parse(ed.src)
	if err != nil {
		return Result{}, oops.
			With("path", path).
			Hint("ai-rulez could not merge its keys into this YAML document without breaking it; the file was left untouched").
			Wrapf(err, "merged YAML document does not parse")
	}
	if err := jsonmerge.VerifyOwned(parsed.tree, owned); err != nil {
		return Result{}, oops.
			With("path", path).
			Hint("ai-rulez could not merge its keys into this YAML document faithfully; the file was left untouched").
			Wrapf(err, "merged YAML document does not hold the owned values")
	}
	if err := jsonmerge.CheckPreservedApply(before.tree, parsed.tree, owned); err != nil {
		return Result{}, oops.
			With("path", path).
			Hint("ai-rulez could not merge its keys into this YAML document without altering the rest of it; the file was left untouched").
			Wrapf(err, "merged YAML document does not preserve the existing content")
	}
	partial := parsed.hasComment || jsonmerge.HasUnownedTree(parsed.tree, jsonmerge.OwnedPaths(owned)) ||
		jsonmerge.HasUserElements(parsed.tree, owned)
	claims := jsonmerge.AnnotateClaims(jsonmerge.ClaimsFor(owned), before.tree, existing)
	return Result{Body: bom + ed.src, PartiallyOwned: partial, Claims: claims}, nil
}

// renderOwned renders a brand-new document containing only the owned keys.
func renderOwned(owned []OwnedKey) (string, error) {
	root := map[string]any{}
	for _, key := range owned {
		segs := key.Segments()
		if len(segs) == 0 || key.Remove {
			continue
		}
		node := root
		for _, seg := range segs[:len(segs)-1] {
			child, ok := node[seg].(map[string]any)
			if !ok {
				child = map[string]any{}
				node[seg] = child
			}
			node = child
		}
		node[segs[len(segs)-1]] = key.Value
	}
	if len(root) == 0 {
		return "", nil
	}
	body, err := marshal(root, defaultIndent)
	if err != nil {
		return "", oops.Wrapf(err, "render YAML document")
	}
	return body, nil
}

// parsed is a document read for editing.
type parsed struct {
	// root is the document's root node, nil for a document with no content.
	root *yaml.Node
	// tree is the document decoded into generic Go values.
	tree       map[string]any
	hasComment bool
}

var (
	errMultipleDocuments = errors.New("the file holds more than one YAML document")
	errNotMapping        = errors.New("the root value is not a mapping")
	errAnchors           = errors.New("the document uses YAML anchors, aliases or merge keys, which ai-rulez cannot edit safely")
	errLineBreak         = errors.New("the document contains a line break other than LF or CRLF " +
		"(a lone CR, NEL, LS or PS), which ai-rulez cannot edit safely")
)

// checkLineBreaks refuses a document with a line break yaml.v3 counts as one but
// the text editor does not, which would put every reported line number out.
func checkLineBreaks(src string) error {
	for i := 0; i < len(src); i++ {
		if src[i] == '\r' && (i+1 >= len(src) || src[i+1] != '\n') {
			return errLineBreak
		}
	}
	if strings.ContainsAny(src, "\u0085\u2028\u2029") {
		return errLineBreak
	}
	return nil
}

func nodeHasAnchor(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	if n.Anchor != "" || n.Kind == yaml.AliasNode {
		return true
	}
	for _, child := range n.Content {
		if nodeHasAnchor(child) {
			return true
		}
	}
	return false
}

// sourceHasComment reports whether any line holds a # that starts a comment (at
// the start of the line or after whitespace). It looks at the text, not the
// nodes yaml.v3 attaches comments to, because it drops some, and a missed
// comment would let a document the user wrote be treated as generated. A # inside
// a quoted value is counted too: wrongly calling a file the user's is the safe
// mistake.
func sourceHasComment(src string) bool {
	for i := 0; i < len(src); i++ {
		if src[i] == '#' && (i == 0 || src[i-1] == ' ' || src[i-1] == '\t' || src[i-1] == '\n' || src[i-1] == '\r') {
			return true
		}
	}
	return false
}

// parse reads a document, requiring a single document whose root is a mapping
// (or nothing at all).
func parse(src string) (*parsed, error) {
	if err := checkLineBreaks(src); err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(strings.NewReader(src))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return &parsed{tree: map[string]any{}, hasComment: strings.Contains(src, "#")}, nil
		}
		return nil, err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, err
		}
		return nil, errMultipleDocuments
	}

	root := &doc
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		root = doc.Content[0]
	}
	if nodeHasAnchor(&doc) {
		return nil, errAnchors
	}
	out := &parsed{tree: map[string]any{}, hasComment: nodeHasComment(&doc) || sourceHasComment(src)}
	switch {
	case root.Kind == yaml.ScalarNode && root.Tag == "!!null":
		return out, nil
	case root.Kind != yaml.MappingNode:
		return nil, errNotMapping
	}
	out.root = root
	if err := root.Decode(&out.tree); err != nil {
		return nil, err
	}
	if out.tree == nil {
		out.tree = map[string]any{}
	}
	return out, nil
}

func nodeHasComment(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	if n.HeadComment != "" || n.LineComment != "" || n.FootComment != "" {
		return true
	}
	for _, child := range n.Content {
		if nodeHasComment(child) {
			return true
		}
	}
	return false
}

func parseFailure(path string, cause error) error {
	hint := path + " is not parseable YAML, and ai-rulez will not overwrite a file it cannot merge into. " +
		"Fix the syntax, or move the file aside."
	if errors.Is(cause, errAnchors) || errors.Is(cause, errLineBreak) {
		hint = path + " uses YAML syntax ai-rulez cannot edit without risking the rest of the file; the file was left untouched."
	}
	if errors.Is(cause, errMultipleDocuments) || errors.Is(cause, errNotMapping) {
		hint = path + " is not a single YAML document with a mapping at its root, which is all ai-rulez can merge into."
	}
	return oops.With("path", path).Hint(hint).Wrapf(cause, "parse existing YAML document")
}

// marshal renders a value or node as block-style YAML with the given indent.
func marshal(value any, indent int) (string, error) {
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(indent)
	if err := enc.Encode(value); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return b.String(), nil
}
