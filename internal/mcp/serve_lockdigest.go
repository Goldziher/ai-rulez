package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// LockDigest is the digest ai-rulez.lock pins for a served skill. It is the
// skill's file digest with one volatile input removed: the Source-Hash line that
// `generate` stamps into every file is a hash of the whole project's sources, so
// editing any other skill would change the digest of every skill. Everything
// that identifies this skill (its text, its frontmatter, its supporting files
// and the per-file Content-Hash) stays in.

var headerCommentPrefixes = []string{"<!--", "-->", "//", "#", ";", "/*", "*/"}

func isSourceHashLine(line string) bool {
	s := strings.TrimSpace(line)
	for _, p := range headerCommentPrefixes {
		s = strings.TrimPrefix(s, p)
	}
	return strings.HasPrefix(strings.TrimSpace(s), "Source-Hash: ")
}

// normalizeForLock drops the Source-Hash header line from the text files of a
// skill (only the leading header region is inspected, never a line deep in a body).
func normalizeForLock(content []byte) []byte {
	const headerLines = 40
	text := string(content)
	lines := strings.SplitAfter(text, "\n")
	var out strings.Builder
	for i, l := range lines {
		if i < headerLines && isSourceHashLine(l) {
			continue
		}
		out.WriteString(l)
	}
	return []byte(out.String())
}

func lockDigest(files []CatalogFile) string {
	type pair struct{ uri, digest string }
	pairs := make([]pair, 0, len(files))
	for i := range files {
		sum := sha256.Sum256(normalizeForLock(files[i].Content))
		pairs = append(pairs, pair{files[i].URI, "sha256:" + hex.EncodeToString(sum[:])})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].uri < pairs[j].uri })
	h := sha256.New()
	for _, p := range pairs {
		h.Write([]byte(p.uri + "\x00" + p.digest + "\n")) //nolint:errcheck // hash.Hash.Write never fails
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
