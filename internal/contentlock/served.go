package contentlock

import (
	"encoding/hex"
	"regexp"
	"strings"
)

// KindServedSkill is the tree kind of a skill served over MCP. The files are the
// rendered skill files (the bytes the server returns), so the digest is
// different from the one of the authored skill with the same name.
const KindServedSkill = "served-skill"

// FileDigest returns the digest of one file, "sha256:<hex>", under the same
// domain-separated scheme as every other pin (line endings of text files are
// normalized).
func FileDigest(l Leaf) string {
	d := leafDigest(l)
	return Algorithm + ":" + hex.EncodeToString(d[:])
}

// ServedDigest returns the digest of the files of a served skill. With
// headerIndependent set, the lines of a generated header that change without the
// skill changing (the project-wide Source-Hash and the Generated stamp) are left
// out first, which is the digest ai-rulez.lock pins.
func ServedDigest(leaves []Leaf, headerIndependent bool) (string, error) {
	if headerIndependent {
		stripped := make([]Leaf, len(leaves))
		for i, l := range leaves {
			stripped[i] = Leaf{Path: l.Path, Mode: l.Mode, Data: StripVolatileHeader(l.Data)}
		}
		leaves = stripped
	}
	return TreeDigest(KindServedSkill, leaves)
}

// volatileHeaderLines is how far into a file the generated header can reach.
const volatileHeaderLines = 40

var (
	headerCommentPrefixes = []string{"<!--", "-->", "//", "#", ";", "/*", "*/"}
	generatedStampPattern = regexp.MustCompile(`(?: \| )?Generated: [^\n]*$`)
)

func hasCommentPrefix(line string) bool {
	s := strings.TrimSpace(line)
	for _, p := range headerCommentPrefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func isSourceHashLine(line string) bool {
	s := strings.TrimSpace(line)
	for _, p := range headerCommentPrefixes {
		s = strings.TrimPrefix(s, p)
	}
	return strings.HasPrefix(strings.TrimSpace(s), "Source-Hash: ")
}

// StripVolatileHeader drops the Source-Hash line and the Generated stamp from
// the generated header at the top of a file. Only the leading header region is
// inspected, and only comment lines: a body line is never touched. The
// Content-Hash line stays, it identifies the file's own content.
func StripVolatileHeader(content []byte) []byte {
	lines := strings.SplitAfter(string(content), "\n")
	var out strings.Builder
	for i, l := range lines {
		if i < volatileHeaderLines && hasCommentPrefix(l) {
			if isSourceHashLine(l) {
				continue
			}
			if strings.Contains(l, "Generated: ") {
				eol := ""
				if strings.HasSuffix(l, "\n") {
					eol = "\n"
				}
				l = generatedStampPattern.ReplaceAllString(strings.TrimSuffix(l, "\n"), "") + eol
			}
		}
		out.WriteString(l)
	}
	return []byte(out.String())
}
