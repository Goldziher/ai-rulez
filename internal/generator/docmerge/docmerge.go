// Package docmerge is the single entry point for merging ai-rulez's keys into a
// shared, user-authored settings document of any supported format: JSON and
// JSONC (jsonmerge), TOML (tomlmerge) and YAML (yamlmerge).
//
// Every engine honors the same contract. Apply replaces only the keys ai-rulez
// owns and leaves the rest of the document, comments included, as it was;
// Unmerge takes back exactly what an earlier Apply claimed. The types are the
// ones jsonmerge defines, so the ownership record (Claim) is the same JSON shape
// whatever the document format and a manifest written for a .json document is
// read back unchanged.
//
// Typical use:
//
//	format, ok := docmerge.FormatFromPath(path)
//	if !ok { /* not a mergeable document */ }
//	result, err := docmerge.Apply(path, format, []docmerge.OwnedKey{
//		{Path: []string{"mcp_servers"}, Value: servers, Members: true},
//	})
//	// write result.Body; keep result.Claims; gitignore only if !result.PartiallyOwned
//
// and later, with the recorded claims:
//
//	unmerged, err := docmerge.Unmerge(path, format, claims)
package docmerge

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/tomlmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/yamlmerge"
	"github.com/samber/oops"
)

// OwnedKey is a key ai-rulez owns, with the value to write. Path (or Name for a
// top-level key) addresses it through nested objects, tables or mappings; Remove
// deletes it; Members merges a map entry by entry; Elements claims only some
// elements of an array. See jsonmerge.OwnedKey for the full semantics.
type OwnedKey = jsonmerge.OwnedKey

// Result is the outcome of Apply: the Body to write, whether the document
// is shared with its consumer (PartiallyOwned) and the Claims to record.
type Result = jsonmerge.Result

// Claim records one thing ai-rulez wrote, addressed by key path. It marshals to
// and from JSON for the generation manifest.
type Claim = jsonmerge.Claim

// Unmerged is the outcome of Unmerge.
type Unmerged = jsonmerge.Unmerged

// Format names a document syntax.
type Format string

const (
	// FormatJSON is a JSON document. Comments and trailing commas in an existing
	// file are detected and preserved, so a .json file that is really JSONC
	// (opencode.json, .vscode/settings.json) needs no special handling.
	FormatJSON Format = "json"
	// FormatJSONC is a JSON-with-comments document; it behaves exactly as
	// FormatJSON.
	FormatJSONC Format = "jsonc"
	// FormatTOML is a TOML document.
	FormatTOML Format = "toml"
	// FormatYAML is a YAML document (one document, block-style root mapping).
	FormatYAML Format = "yaml"
)

// FormatMarkdown, a Markdown document ai-rulez owns only blocks of, is declared
// in markdown.go.

// FormatFromPath infers the format from a file extension: .json, .jsonc, .toml,
// .yaml and .yml. ok is false for any other extension, .md included: a Markdown
// file is mergeable only where the caller knows it holds ai-rulez blocks.
func FormatFromPath(path string) (format Format, ok bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return FormatJSON, true
	case ".jsonc":
		return FormatJSONC, true
	case ".toml":
		return FormatTOML, true
	case ".yaml", ".yml":
		return FormatYAML, true
	default:
		return "", false
	}
}

// Valid reports whether the format is one docmerge can merge into.
func (f Format) Valid() bool {
	switch f {
	case FormatJSON, FormatJSONC, FormatTOML, FormatYAML, FormatMarkdown:
		return true
	default:
		return false
	}
}

// Apply merges the owned keys into the document at path, which is parsed as
// format, and returns the body to write. A missing or empty file is created from
// the owned keys alone. A document that cannot be parsed, or whose shape the
// merge cannot honor, is an error and the file is not touched (Apply never
// writes).
//
// An empty Result.Body means there is nothing to write: the file is missing (or
// empty) and no owned key writes a value (every one is a Remove). Callers must
// not create a zero-byte file for it, in any format.
//
// An Elements key claims some elements of an array the document may share with
// its user, and the caller is responsible for it: Value must be the whole array
// to write (the user's elements and ai-rulez's together), because Apply replaces
// the array with it and cannot tell which elements an earlier run added and this
// one wants gone. A document whose Elements array holds an element that is not
// among the claimed ones is reported PartiallyOwned, and an array in the style
// the document already uses (TOML inline array of tables, YAML flow sequence)
// keeps it. Members and Elements on one key is an error.
//
// Every engine verifies its own edit before returning it: the result must parse,
// hold every owned value, and differ from the original in nothing but the owned
// keys; otherwise Apply fails and the file is left as it was. A UTF-8 byte order
// mark is kept, and a document that had no final newline gets one from Apply,
// which the claims record so Unmerge takes it back out.
func Apply(path string, format Format, owned []OwnedKey) (Result, error) {
	return ApplyWith(os.ReadFile, path, format, owned)
}

// ApplyWith is Apply reading the existing document through read.
func ApplyWith(read jsonmerge.Reader, path string, format Format, owned []OwnedKey) (Result, error) {
	result, err := apply(read, path, format, owned)
	result.Owned = owned
	return result, err
}

func apply(read jsonmerge.Reader, path string, format Format, owned []OwnedKey) (Result, error) {
	if !format.Valid() {
		return Result{}, unsupported(format)
	}
	existing, found, err := jsonmerge.ReadExistingWith(read, path)
	if err != nil {
		return Result{}, err
	}
	if !found {
		return create(format, owned)
	}
	return ApplyDocument(path, format, existing, owned)
}

// ApplyDocument is Apply for a document already read; path only names it in
// errors.
func ApplyDocument(path string, format Format, existing string, owned []OwnedKey) (Result, error) {
	result, err := applyDocument(path, format, existing, owned)
	result.Owned = owned
	return result, err
}

func applyDocument(path string, format Format, existing string, owned []OwnedKey) (Result, error) {
	if !format.Valid() {
		return Result{}, unsupported(format)
	}
	if strings.TrimSpace(existing) == "" {
		return create(format, owned)
	}
	if err := jsonmerge.ValidateOwned(owned); err != nil {
		return Result{}, oops.With("path", path).Wrapf(err, "invalid owned keys")
	}
	switch format {
	case FormatJSON, FormatJSONC:
		return jsonmerge.ApplyDocument(path, existing, owned)
	case FormatTOML:
		return tomlmerge.ApplyDocument(path, existing, owned)
	case FormatMarkdown:
		return applyMarkdown(path, existing, owned)
	default:
		return yamlmerge.ApplyDocument(path, existing, owned)
	}
}

// create builds a document from the owned keys alone, with an empty body when
// none of them writes anything.
func create(format Format, owned []OwnedKey) (Result, error) {
	if err := jsonmerge.ValidateOwned(owned); err != nil {
		return Result{}, oops.Wrapf(err, "invalid owned keys")
	}
	var result Result
	var err error
	switch format {
	case FormatJSON, FormatJSONC:
		result, err = jsonmerge.Apply("", owned)
	case FormatTOML:
		result, err = tomlmerge.Apply("", owned)
	case FormatMarkdown:
		result, err = applyMarkdown("", "", owned)
	default:
		result, err = yamlmerge.Apply("", owned)
	}
	if err == nil && !slices.ContainsFunc(owned, func(k OwnedKey) bool { return !k.Remove && len(k.Segments()) > 0 }) {
		result.Body = ""
	}
	return result, err
}

// Unmerge removes the claimed content from the document at path, leaving
// everything else as Apply does. Content that is absent, or whose value the user
// has since edited, is left alone (see Unmerged.Kept). A missing file changes
// nothing.
func Unmerge(path string, format Format, claims []Claim) (Unmerged, error) {
	return UnmergeWith(os.ReadFile, path, format, claims)
}

// UnmergeWith is Unmerge reading the document through read.
func UnmergeWith(read jsonmerge.Reader, path string, format Format, claims []Claim) (Unmerged, error) {
	if !format.Valid() {
		return Unmerged{}, unsupported(format)
	}
	existing, found, err := jsonmerge.ReadExistingWith(read, path)
	if err != nil || !found {
		return Unmerged{}, err
	}
	return UnmergeDocument(path, format, existing, claims)
}

// UnmergeDocument is Unmerge for a document already read; path only names it in
// errors.
func UnmergeDocument(path string, format Format, existing string, claims []Claim) (Unmerged, error) {
	switch format {
	case FormatJSON, FormatJSONC:
		return jsonmerge.UnmergeDocument(path, existing, claims)
	case FormatTOML:
		return withoutHashHeader(tomlmerge.UnmergeDocument(path, existing, claims))
	case FormatYAML:
		return withoutHashHeader(yamlmerge.UnmergeDocument(path, existing, claims))
	case FormatMarkdown:
		return unmergeMarkdown(path, existing, claims)
	default:
		return Unmerged{}, unsupported(format)
	}
}

// withoutHashHeader drops the Content-Hash and Source-Hash lines generate stamps
// into the leading comment block of a merged document, so that taking ai-rulez's
// content back out leaves the file as the user wrote it.
func withoutHashHeader(result Unmerged, err error) (Unmerged, error) {
	if err != nil || !result.Changed || result.Body == "" {
		return result, err
	}
	var out strings.Builder
	inHeader := true
	for _, line := range strings.SplitAfter(result.Body, "\n") {
		if inHeader {
			trimmed := strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(trimmed, "# Content-Hash: "), strings.HasPrefix(trimmed, "# Source-Hash: "):
				continue
			case !strings.HasPrefix(trimmed, "#"):
				inHeader = false
			}
		}
		out.WriteString(line)
	}
	result.Body = out.String()
	return result, nil
}

func unsupported(format Format) error {
	return oops.
		With("format", string(format)).
		Hint("supported formats are json, jsonc, toml and yaml").
		Errorf("unsupported document format %q", string(format))
}

// DecodeTree decodes a document of the given format into generic Go values, for
// reading what it already holds. Markdown has no key tree and is an error.
func DecodeTree(format Format, doc string) (map[string]any, error) {
	switch format {
	case FormatJSON, FormatJSONC:
		return jsonmerge.DecodeTolerantTree(doc)
	case FormatTOML:
		return tomlmerge.DecodeTree(doc)
	case FormatYAML:
		return yamlmerge.DecodeTree(doc)
	default:
		return nil, unsupported(format)
	}
}
