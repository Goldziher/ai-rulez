package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/samber/oops"
	"github.com/zeebo/blake3"
)

const (
	provenanceFileName = ".ai-rulez-generated.json"
	provenanceSchema   = "v1"
)

type provenanceOutput struct {
	ContentHash string `json:"content_hash"`
}

type provenanceDocument struct {
	SchemaVersion string                      `json:"schema_version"`
	SourceHash    string                      `json:"source_hash"`
	Outputs       map[string]provenanceOutput `json:"outputs"`
	// Includes lists the bundle-relative paths copied verbatim by [plugin]
	// include, sorted. The publish emitter reads it to carry those files into the
	// Agent Plugins package without re-reading the configuration.
	Includes []string `json:"includes,omitempty"`
	// Obsolete records generated files a run kept because they were edited (or
	// cannot be deleted safely) but no longer plans. They stay owned, so later
	// verify and generate runs keep reporting them until they are reconciled or
	// deleted by hand.
	Obsolete map[string]provenanceOutput `json:"obsolete,omitempty"`
}

// AddProvenance adds deterministic generated-file headers where the target
// format supports comments and records every output in a strict-JSON sidecar.
// JSON and binary artifacts remain byte-identical to keep runtime schemas valid.
func AddProvenance(outputs []config.OutputFile, baseDir string) ([]config.OutputFile, error) {
	entries := make(map[string]provenanceOutput, len(outputs))
	paths := make([]string, 0, len(outputs))
	var includes []string
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		relativePath, err := filepath.Rel(baseDir, output.Path)
		if err != nil || safefs.RelEscapes(relativePath) {
			return nil, oops.With("path", output.Path).With("base_dir", baseDir).Errorf("plugin output escapes bundle root")
		}
		relativePath = filepath.ToSlash(relativePath)
		entries[relativePath] = provenanceOutput{ContentHash: hashBytes(outputBytes(output))}
		paths = append(paths, relativePath)
		if output.PluginInclude {
			includes = append(includes, relativePath)
		}
	}
	sort.Strings(paths)
	sort.Strings(includes)

	var sourceInput strings.Builder
	sourceInput.WriteString("schema=" + provenanceSchema + "\n")
	for _, path := range paths {
		sourceInput.WriteString(path + "=" + entries[path].ContentHash + "\n")
	}
	sourceHash := hashBytes([]byte(sourceInput.String()))

	decorated := make([]config.OutputFile, 0, len(outputs)+1)
	for _, output := range outputs {
		body := outputBytes(output)
		relativePath, err := filepath.Rel(baseDir, output.Path)
		if err != nil {
			return nil, oops.With("path", output.Path).Wrapf(err, "resolve plugin provenance path")
		}
		// An included file is copied verbatim, so it takes no generated-file
		// header; its hash in the sidecar is the source's own.
		if !output.PluginInclude {
			if header := provenanceHeader(output.Path, entries[filepath.ToSlash(relativePath)].ContentHash, sourceHash); header != "" {
				body = insertProvenanceHeader(body, output.Path, header)
			}
		}
		output.Content = ""
		output.RawContent = body
		decorated = append(decorated, output)
	}

	sidecar, err := jsonOutput(filepath.Join(baseDir, provenanceFileName), provenanceDocument{
		SchemaVersion: provenanceSchema,
		SourceHash:    sourceHash,
		Outputs:       entries,
		Includes:      includes,
	})
	if err != nil {
		return nil, err
	}
	return append(decorated, sidecar), nil
}

// RetainObsolete carries the obsolete records of the previous sidecar, plus the
// files a run newly kept, into the planned provenance sidecar. Later verify and
// generate runs still recognize those files as ai-rulez-generated and keep
// reporting them until they are reconciled into the sources or deleted by hand. A
// record whose path the plan contains again is dropped. The planned sidecar is
// returned unchanged when there is nothing to retain.
func RetainObsolete(plannedSidecar, previousSidecar []byte, kept []Obsolete) ([]byte, error) {
	var document provenanceDocument
	if err := unmarshalProvenance(plannedSidecar, &document); err != nil {
		return nil, err
	}
	records := make(map[string]provenanceOutput)
	if len(previousSidecar) > 0 {
		var previous provenanceDocument
		if err := unmarshalProvenance(previousSidecar, &previous); err != nil {
			return nil, err
		}
		for rel, recorded := range previous.Obsolete {
			records[rel] = recorded
		}
	}
	for _, item := range kept {
		records[filepath.ToSlash(item.Rel)] = provenanceOutput{ContentHash: item.ContentHash}
	}
	for rel := range records {
		if _, planned := document.Outputs[rel]; planned {
			delete(records, rel)
		}
	}
	if len(records) == 0 {
		return plannedSidecar, nil
	}
	if document.Obsolete == nil {
		document.Obsolete = make(map[string]provenanceOutput, len(records))
	}
	for rel, recorded := range records {
		document.Obsolete[rel] = recorded
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, oops.Wrapf(err, "marshal plugin provenance")
	}
	return append(data, '\n'), nil
}

func outputBytes(output config.OutputFile) []byte {
	if output.RawContent != nil {
		return output.RawContent
	}
	return []byte(output.Content)
}

func hashBytes(content []byte) string {
	hash := blake3.Sum256(content)
	return fmt.Sprintf("blake3:%x", hash)
}

func provenanceHeader(path, contentHash, sourceHash string) string {
	lines := []string{
		"AI-RULEZ :: GENERATED FILE — DO NOT EDIT",
		"Content-Hash: " + contentHash,
		"Source-Hash: " + sourceHash,
		"Schema-Version: " + provenanceSchema,
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".mdx", ".markdown":
		return "<!--\n" + strings.Join(lines, "\n") + "\n-->\n"
	case ".js", ".jsx", ".ts", ".tsx":
		return "// " + strings.Join(lines, "\n// ") + "\n"
	case ".py":
		return "# " + strings.Join(lines, "\n# ") + "\n"
	default:
		return ""
	}
}

func insertProvenanceHeader(body []byte, path, header string) []byte {
	content := string(body)
	extension := strings.ToLower(filepath.Ext(path))
	if (extension == ".md" || extension == ".mdx" || extension == ".markdown") && strings.HasPrefix(content, "---\n") {
		rest := content[len("---\n"):]
		if index := strings.Index(rest, "\n---\n"); index >= 0 {
			closing := len("---\n") + index + len("\n---\n")
			return []byte(content[:closing] + "\n" + header + content[closing:])
		}
	}
	if strings.HasPrefix(content, "#!") {
		if index := strings.IndexByte(content, '\n'); index >= 0 {
			return []byte(content[:index+1] + header + content[index+1:])
		}
	}
	return []byte(header + "\n" + content)
}

// ProvenanceFileName is the sidecar every generated bundle root carries.
const ProvenanceFileName = provenanceFileName

// ProvenanceOutputs decodes a provenance sidecar into the content hash of each
// bundle file it records, keyed by bundle-relative path, plus the bundle's
// source hash.
func ProvenanceOutputs(sidecar []byte) (hashes map[string]string, sourceHash string, err error) {
	var document provenanceDocument
	if err := json.Unmarshal(sidecar, &document); err != nil {
		return nil, "", oops.Wrapf(err, "parse plugin provenance")
	}
	hashes = make(map[string]string, len(document.Outputs))
	for path, out := range document.Outputs {
		hashes[path] = out.ContentHash
	}
	return hashes, document.SourceHash, nil
}

// ProvenanceIncludes returns the bundle-relative paths a provenance sidecar
// records as copied verbatim by [plugin] include.
func ProvenanceIncludes(sidecar []byte) ([]string, error) {
	var document provenanceDocument
	if err := json.Unmarshal(sidecar, &document); err != nil {
		return nil, oops.Wrapf(err, "parse plugin provenance")
	}
	return document.Includes, nil
}

func unmarshalProvenance(sidecar []byte, document *provenanceDocument) error {
	if err := json.Unmarshal(sidecar, document); err != nil {
		return oops.Wrapf(err, "parse plugin provenance")
	}
	return nil
}

// provenanceHeaderPattern matches the comment block provenanceHeader writes, in
// each of the comment syntaxes: HTML, // and #.
var provenanceHeaderPattern = regexp.MustCompile(
	`(?m)^(?:<!--\n|// |# )AI-RULEZ :: GENERATED FILE — DO NOT EDIT\n` +
		`(?://\s|#\s)?Content-Hash: [^\n]*\n(?://\s|#\s)?Source-Hash: [^\n]*\n(?://\s|#\s)?Schema-Version: [^\n]*\n(?:-->\n)?`)

// StripProvenance removes the generated-file header AddProvenance inserted into
// body, undoing insertProvenanceHeader exactly, so a file read back from a
// generated bundle is the source it was generated from. Files that never take a
// header, and files without one, are returned as they are.
func StripProvenance(path string, body []byte) []byte {
	if provenanceHeader(path, "", "") == "" {
		return body
	}
	loc := provenanceHeaderPattern.FindIndex(body)
	if loc == nil {
		return body
	}
	head, tail := body[:loc[0]], body[loc[1]:]
	switch {
	case len(head) == 0 && bytes.HasPrefix(tail, []byte("\n")):
		// Inserted at the top: header, a blank line, then the content.
		return tail[1:]
	case bytes.HasSuffix(head, []byte("\n\n")) && !bytes.HasPrefix(body, []byte("#!")):
		// Inserted after the front matter: a blank line, then the header.
		return slices.Concat(head[:len(head)-1], tail)
	}
	return slices.Concat(head, tail)
}
