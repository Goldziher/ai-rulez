package publish

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/samber/oops"
)

// SchemaVersion is the schema_version of the manifest and the publish plan.
const SchemaVersion = 1

// Digest returns "sha256:<hex>" of data.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Manifest describes one published bundle (schema/publish-manifest.schema.json).
// Approval, Signature and SBOM are null until those features exist; the schema
// reserves them so a consumer can refuse unsigned or unapproved bundles later.
type Manifest struct {
	SchemaVersion int         `json:"schema_version"`
	Name          string      `json:"name"`
	Version       string      `json:"version"`
	AIRulezVer    string      `json:"ai_rulez_version"`
	Source        Source      `json:"source"`
	Lock          LockInfo    `json:"lock"`
	Runtimes      []string    `json:"runtimes"`
	Files         []FileEntry `json:"files"`
	Bundle        BundleInfo  `json:"bundle"`
	Approval      any         `json:"approval"`
	Signature     any         `json:"signature"`
	SBOM          any         `json:"sbom"`
}

// Source is where the bundle came from. Repo never carries credentials.
type Source struct {
	Repo   string `json:"repo"`
	Commit string `json:"commit"`
	Dirty  bool   `json:"dirty"`
}

// LockInfo ties the bundle to ai-rulez.lock.
type LockInfo struct {
	Version    int    `json:"version"`
	Tree       string `json:"tree"`
	FileDigest string `json:"file_digest"`
}

// FileEntry is one file of the bundle.
type FileEntry struct {
	Path   string `json:"path"`
	Size   int    `json:"size"`
	Digest string `json:"digest"`
}

// BundleInfo describes the archive.
type BundleInfo struct {
	File   string `json:"file"`
	Digest string `json:"digest"`
	Size   int    `json:"size"`
}

// marshalJSON encodes v with two-space indentation, a trailing newline and no
// HTML escaping, so the bytes depend only on the data.
func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, oops.Wrapf(err, "encode json")
	}
	return buf.Bytes(), nil
}

// Marshal returns the manifest document.
func (m Manifest) Marshal() ([]byte, error) { return marshalJSON(m) }

// SumEntry is one line of SHA256SUMS.
type SumEntry struct {
	Path   string
	Digest string // "sha256:<hex>"
}

// FormatSums renders SHA256SUMS in the `sha256sum` format ("<hex>  <path>"),
// sorted by path.
func FormatSums(entries []SumEntry) []byte {
	sorted := append([]SumEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	var sb strings.Builder
	for _, e := range sorted {
		sb.WriteString(strings.TrimPrefix(e.Digest, "sha256:"))
		sb.WriteString("  ")
		sb.WriteString(e.Path)
		sb.WriteByte('\n')
	}
	return []byte(sb.String())
}

// ParseSums reads SHA256SUMS. Every line must be "<64 hex>  <path>" with a valid path.
func ParseSums(data []byte) ([]SumEntry, error) {
	var out []SumEntry
	for i, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if line == "" {
			return nil, oops.Errorf("SHA256SUMS line %d is empty", i+1)
		}
		hexSum, name, ok := strings.Cut(line, "  ")
		if !ok || len(hexSum) != sha256.Size*2 || !ValidPath(name) {
			return nil, oops.Errorf("SHA256SUMS line %d is not \"<sha256>  <path>\"", i+1)
		}
		if _, err := hex.DecodeString(hexSum); err != nil || strings.ToLower(hexSum) != hexSum {
			return nil, oops.Errorf("SHA256SUMS line %d has an invalid digest", i+1)
		}
		out = append(out, SumEntry{Path: name, Digest: "sha256:" + hexSum})
	}
	return out, nil
}
