package lint

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Fingerprints identify a finding across edits that move it: they are derived
// from the rule code, the repository-relative path and the normalized text of
// the line the finding points at, never from the line number. Baselines and
// SARIF partialFingerprints use them.

const fingerprintVersion = "ar1"

// normalizeText trims a line and collapses runs of whitespace, so re-indenting
// or re-wrapping trailing spaces does not change a fingerprint.
func normalizeText(line string) string {
	return strings.Join(strings.Fields(line), " ")
}

// fingerprintOf hashes one finding's identity. occurrence separates findings
// that share the same code, path and text (the second broken link on identical
// lines).
func fingerprintOf(code, path, text string, occurrence int) string {
	h := sha256.New()
	for _, part := range []string{fingerprintVersion, code, path, normalizeText(text)} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	h.Write([]byte(strconv.Itoa(occurrence)))
	return fingerprintVersion + ":" + hex.EncodeToString(h.Sum(nil))[:24]
}

// metricBucket groups a measurement into steps of 25% growth (1, 2, 3, 4, 5,
// 7, 9, 11, ...), so a size finding keeps its fingerprint while the file
// wobbles by a few lines and gets a new one once it has clearly grown.
func metricBucket(n int) int {
	bucket := 0
	for v := 1.0; v*1.25 <= float64(n); v *= 1.25 {
		bucket++
	}
	return bucket
}

// assignIdentity fills Path and Fingerprint on findings sorted by file, line
// and code. abs resolves a display path to a file on disk.
func assignIdentity(findings []Finding, tree *Tree, cwd string) {
	lines := map[string][]string{}
	seen := map[string]int{}
	for i := range findings {
		f := &findings[i]
		abs := f.File
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, filepath.FromSlash(abs))
		}
		path := filepath.ToSlash(f.File)
		if tree != nil {
			if rel := tree.Rel(abs); rel != "" {
				path = rel
			}
		}
		if label, ok := includeCacheLabel(path); ok {
			path = label // the cache sits under the home directory, which differs per machine
		}
		f.meta().Path = path
		annotateAnalyzer(f)
		ls, ok := lines[abs]
		if !ok {
			if data, err := os.ReadFile(abs); err == nil {
				ls = strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
			}
			lines[abs] = ls
		}
		text := f.Message // unreadable file (a synthetic location): the message is the identity
		if f.Line >= 1 && f.Line <= len(ls) {
			text = ls[f.Line-1]
		}
		if f.Meta.Metric > 0 {
			// The flagged line is a generic anchor (the frontmatter opener);
			// the size bucket is what distinguishes one measurement from another.
			text += "\x00size-bucket:" + strconv.Itoa(metricBucket(f.Meta.Metric))
		}
		key := f.Code + "\x00" + path + "\x00" + normalizeText(text)
		if f.meta().Fingerprint == "" { // an external scanner's finding arrives with its own
			f.meta().Fingerprint = fingerprintOf(f.Code, path, text, seen[key])
		}
		seen[key]++
	}
}
