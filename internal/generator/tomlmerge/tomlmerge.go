// Package tomlmerge rewrites the keys ai-rulez owns inside a user-authored TOML
// document (.codex/config.toml, .grok/config.toml, reasonix.toml, ...) without
// disturbing anything else in it.
//
// It follows the contract of jsonmerge and shares its types: OwnedKey, Result,
// Claim and Unmerged. A key path addresses a dotted key, so {"mcp_servers",
// "foo"} is the table [mcp_servers.foo] (or a key inside whichever table the
// document already uses); Members merges a map entry by entry, which is how an
// MCP server map is written without touching servers the user added.
//
// The document is edited as text, not re-encoded: the engine locates the owned
// tables and keys by byte span and replaces, inserts or removes just those, so
// comments, blank lines, ordering and formatting of everything else survive
// byte for byte. Every result is parsed again before it is returned, so an edit
// that would produce invalid TOML (extending an inline table, say) is an error
// rather than a corrupted file.
package tomlmerge

import (
	"strings"

	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
)

// OwnedKey is a key ai-rulez owns, shared with jsonmerge. See jsonmerge.OwnedKey.
type OwnedKey = jsonmerge.OwnedKey

// Result is the outcome of one merge, shared with jsonmerge.
type Result = jsonmerge.Result

// Claim records what ai-rulez wrote, shared with jsonmerge.
type Claim = jsonmerge.Claim

// Unmerged is the outcome of Unmerge, shared with jsonmerge.
type Unmerged = jsonmerge.Unmerged

// Apply merges the owned keys into the TOML document at path, leaving every
// other statement as it was. When path is empty or no file is there yet, the
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
	before, err := parseTree(existing)
	if err != nil {
		return Result{}, parseFailure(path, err)
	}
	ed := &editor{src: existing, newline: detectLineEnding(existing)}
	for _, key := range owned {
		segs := key.Segments()
		if len(segs) == 0 {
			continue
		}
		if err := ed.apply(key, segs); err != nil {
			return Result{}, oops.With("path", path).Wrapf(err, "merge owned keys into TOML document")
		}
	}

	tree, err := parseTree(ed.src)
	if err != nil {
		return Result{}, oops.
			With("path", path).
			Hint("ai-rulez could not merge its keys into this TOML document without breaking it; the file was left untouched").
			Wrapf(err, "merged TOML document does not parse")
	}
	if err := jsonmerge.VerifyOwned(tree, owned); err != nil {
		return Result{}, oops.
			With("path", path).
			Hint("ai-rulez could not merge its keys into this TOML document faithfully; the file was left untouched").
			Wrapf(err, "merged TOML document does not hold the owned values")
	}
	if err := jsonmerge.CheckPreservedApply(before, tree, owned); err != nil {
		return Result{}, oops.
			With("path", path).
			Hint("ai-rulez could not merge its keys into this TOML document without altering the rest of it; the file was left untouched").
			Wrapf(err, "merged TOML document does not preserve the existing content")
	}
	doc, err := scan(ed.src)
	if err != nil {
		return Result{}, oops.With("path", path).Wrapf(err, "scan merged TOML document")
	}
	partial := doc.hasComment || jsonmerge.HasUnownedTree(tree, jsonmerge.OwnedPaths(owned)) ||
		jsonmerge.HasUserElements(tree, owned)
	claims := jsonmerge.AnnotateClaims(jsonmerge.ClaimsFor(owned), before, existing)
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
	body, err := renderDocument(root)
	if err != nil {
		return "", oops.Wrapf(err, "render TOML document")
	}
	return body, nil
}

// parseTree decodes a document into generic Go values.
func parseTree(src string) (map[string]any, error) {
	tree := map[string]any{}
	if err := toml.Unmarshal([]byte(src), &tree); err != nil {
		return nil, err
	}
	return tree, nil
}

func parseFailure(path string, cause error) error {
	return oops.
		With("path", path).
		Hint(path+" is not parseable TOML, and ai-rulez will not overwrite a file it cannot merge into. "+
			"Fix the syntax, or move the file aside.").
		Wrapf(cause, "parse existing TOML document")
}

func detectLineEnding(doc string) string {
	if strings.Contains(doc, "\r\n") {
		return "\r\n"
	}
	return "\n"
}
