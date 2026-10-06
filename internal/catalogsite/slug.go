package catalogsite

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

const (
	maxSlugLen = 100
	hashLen    = 8
)

// windowsReserved are device names a file may not be called on Windows.
var windowsReserved = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true, "com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// slugger turns source text into file-name safe, collision free path segments.
// It is deterministic: the same inputs in the same order give the same names.
type slugger struct{ taken map[string]bool }

func newSlugger() *slugger { return &slugger{taken: map[string]bool{}} }

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:hashLen]
}

// segment slugs one path segment. A segment that had to be altered (anything
// but [a-z0-9._-] in the input, a reserved name, an over-long name) carries a
// hash of the original so two different names never share a file.
func segment(s string) string {
	var b strings.Builder
	lossy := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			lossy = true
			if r >= 'A' && r <= 'Z' {
				b.WriteRune(r + ('a' - 'A'))
			} else if !strings.HasSuffix(b.String(), "-") {
				b.WriteByte('-')
			}
		}
	}
	out := strings.Trim(b.String(), ".")
	if out != b.String() {
		lossy = true
	}
	if out == "" {
		out, lossy = "x", true
	}
	if windowsReserved[strings.SplitN(out, ".", 2)[0]] {
		lossy = true
	}
	if len(out) > maxSlugLen {
		out, lossy = out[:maxSlugLen], true
	}
	if lossy {
		out += "-" + shortHash(s)
	}
	return out
}

// path joins slugged segments under prefix and ".html"; if the result collides
// (compared case-insensitively, as macOS and Windows file systems do) with an
// earlier page, a hash of the original key is appended.
func (g *slugger) path(prefix string, segments []string, key string) string {
	parts := make([]string, len(segments))
	for i, s := range segments {
		parts[i] = segment(s)
	}
	name := prefix + strings.Join(parts, "/")
	candidate := name + ".html"
	for n := 0; g.taken[strings.ToLower(candidate)]; n++ {
		candidate = name + "-" + shortHash(key+"#"+strconv.Itoa(n)) + ".html"
	}
	g.taken[strings.ToLower(candidate)] = true
	return candidate
}
