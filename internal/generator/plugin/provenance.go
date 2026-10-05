package plugin

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
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
}

// AddProvenance adds deterministic generated-file headers where the target
// format supports comments and records every output in a strict-JSON sidecar.
// JSON and binary artifacts remain byte-identical to keep runtime schemas valid.
func AddProvenance(outputs []config.OutputFile, baseDir string) ([]config.OutputFile, error) {
	entries := make(map[string]provenanceOutput, len(outputs))
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		relativePath, err := filepath.Rel(baseDir, output.Path)
		if err != nil || strings.HasPrefix(relativePath, "..") {
			return nil, oops.With("path", output.Path).With("base_dir", baseDir).Errorf("plugin output escapes bundle root")
		}
		relativePath = filepath.ToSlash(relativePath)
		entries[relativePath] = provenanceOutput{ContentHash: hashBytes(outputBytes(output))}
	}
	sourceHash := provenanceSourceHash(entries)

	decorated := make([]config.OutputFile, 0, len(outputs)+1)
	for _, output := range outputs {
		body := outputBytes(output)
		relativePath, err := filepath.Rel(baseDir, output.Path)
		if err != nil {
			return nil, oops.With("path", output.Path).Wrapf(err, "resolve plugin provenance path")
		}
		if header := provenanceHeader(output.Path, entries[filepath.ToSlash(relativePath)].ContentHash, sourceHash); header != "" {
			body = insertProvenanceHeader(body, output.Path, header)
		}
		output.Content = ""
		output.RawContent = body
		decorated = append(decorated, output)
	}

	sidecar, err := jsonOutput(filepath.Join(baseDir, provenanceFileName), provenanceDocument{
		SchemaVersion: provenanceSchema,
		SourceHash:    sourceHash,
		Outputs:       entries,
	})
	if err != nil {
		return nil, err
	}
	sidecar.PluginInventory = true
	return append(decorated, sidecar), nil
}

func provenanceSourceHash(outputs map[string]provenanceOutput) string {
	paths := make([]string, 0, len(outputs))
	for path := range outputs {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var sourceInput strings.Builder
	sourceInput.WriteString("schema=" + provenanceSchema + "\n")
	for _, path := range paths {
		sourceInput.WriteString(path + "=" + outputs[path].ContentHash + "\n")
	}
	return hashBytes([]byte(sourceInput.String()))
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

func unmarshalProvenance(sidecar []byte, document *provenanceDocument) error {
	if err := json.Unmarshal(sidecar, document); err != nil {
		return oops.Wrapf(err, "parse plugin provenance")
	}
	if document.SchemaVersion != provenanceSchema {
		return oops.Errorf("unsupported plugin provenance schema %q", document.SchemaVersion)
	}
	if document.Outputs == nil || provenanceSourceHash(document.Outputs) != document.SourceHash {
		return oops.Errorf("invalid plugin provenance inventory or source hash")
	}
	return nil
}
