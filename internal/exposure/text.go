package exposure

import (
	"path"
	"strings"
	"unicode/utf8"
)

// decodeText reads file bytes as UTF-8, each invalid byte becoming U+FFFD.
func decodeText(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}
	var b strings.Builder
	b.Grow(len(data))
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		b.WriteRune(r)
		data = data[size:]
	}
	return b.String()
}

// splitLines splits text at every line boundary a Python str.splitlines
// recognizes (LF, CR, CRLF, VT, FF, FS, GS, RS, NEL, LS, PS); a final
// terminator does not start another line. The body of a CLAUDE.md or rule is
// rejoined from these lines, which keeps the text hash of the reference
// implementation.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch r {
		case '\n', '\v', '\f', '\x1c', '\x1d', '\x1e', '\u0085', '\u2028', '\u2029':
			lines = append(lines, s[start:i])
			i += size
		case '\r':
			lines = append(lines, s[start:i])
			i += size
			if i < len(s) && s[i] == '\n' {
				i++
			}
		default:
			i += size
			continue
		}
		start = i
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// pySuffix is the extension of a file name as Python's PurePath.suffix has
// it: empty for a dotfile such as ".md" or a name ending in a dot.
func pySuffix(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 || i == len(name)-1 {
		return ""
	}
	return name[i:]
}

// pyWithoutSuffix drops the extension of the last element of a slash path.
func pyWithoutSuffix(p string) string {
	dir, base := path.Split(p)
	return dir + strings.TrimSuffix(base, pySuffix(base))
}
