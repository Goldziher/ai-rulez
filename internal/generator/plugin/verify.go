package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"
)

// VerifyProvenance verifies every generated output recorded in a plugin
// bundle's provenance sidecar. Comment-safe generated headers are removed
// before hashing because the sidecar records the original runtime payload.
func VerifyProvenance(baseDir string) error {
	sidecarPath := filepath.Join(baseDir, provenanceFileName)
	data, err := os.ReadFile(sidecarPath)
	if err != nil {
		return oops.With("path", sidecarPath).Wrapf(err, "read plugin provenance")
	}

	var document provenanceDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return oops.With("path", sidecarPath).Wrapf(err, "parse plugin provenance")
	}
	if document.SchemaVersion != provenanceSchema {
		return oops.With("schema_version", document.SchemaVersion).
			Errorf("unsupported plugin provenance schema")
	}

	paths := make([]string, 0, len(document.Outputs))
	for path, expected := range document.Outputs {
		outputPath, err := safeOutputPath(baseDir, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(outputPath)
		if err != nil {
			return oops.With("path", path).Wrapf(err, "read generated plugin output")
		}
		body = removeProvenanceHeader(body, outputPath, expected.ContentHash, document.SourceHash)
		if actual := hashBytes(body); actual != expected.ContentHash {
			return oops.Errorf(
				"generated plugin output %q hash mismatch: expected %s, got %s",
				path,
				expected.ContentHash,
				actual,
			)
		}
		paths = append(paths, filepath.ToSlash(path))
	}

	sort.Strings(paths)
	var sourceInput strings.Builder
	sourceInput.WriteString("schema=" + provenanceSchema + "\n")
	for _, path := range paths {
		sourceInput.WriteString(path + "=" + document.Outputs[path].ContentHash + "\n")
	}
	if actual := hashBytes([]byte(sourceInput.String())); actual != document.SourceHash {
		return oops.With("expected", document.SourceHash).
			With("actual", actual).
			Errorf("plugin provenance source hash mismatch")
	}
	return nil
}

func safeOutputPath(baseDir, relativePath string) (string, error) {
	if relativePath == "" || filepath.IsAbs(relativePath) {
		return "", oops.With("path", relativePath).Errorf("invalid plugin provenance output path")
	}
	cleanPath := filepath.Clean(filepath.FromSlash(relativePath))
	if cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return "", oops.With("path", relativePath).Errorf("plugin provenance output escapes bundle root")
	}
	return filepath.Join(baseDir, cleanPath), nil
}

func removeProvenanceHeader(body []byte, path, contentHash, sourceHash string) []byte {
	header := provenanceHeader(path, contentHash, sourceHash)
	if header == "" {
		return body
	}
	content := string(body)
	if strings.HasPrefix(content, header+"\n") {
		return []byte(strings.TrimPrefix(content, header+"\n"))
	}
	if strings.HasPrefix(content, "#!") {
		if index := strings.IndexByte(content, '\n'); index >= 0 && strings.HasPrefix(content[index+1:], header) {
			return []byte(content[:index+1] + strings.TrimPrefix(content[index+1:], header))
		}
	}
	marker := "\n" + header
	if index := strings.Index(content, marker); index >= 0 {
		return []byte(content[:index] + content[index+len(marker):])
	}
	return body
}
