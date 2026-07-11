package plugin

import (
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
	paths := make([]string, 0, len(outputs))
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
		paths = append(paths, relativePath)
	}
	sort.Strings(paths)

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
	return append(decorated, sidecar), nil
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
			return []byte(content[:closing] + "\n" + header + "\n" + content[closing:])
		}
	}
	if strings.HasPrefix(content, "#!") {
		if index := strings.IndexByte(content, '\n'); index >= 0 {
			return []byte(content[:index+1] + header + content[index+1:])
		}
	}
	return []byte(header + "\n" + content)
}
