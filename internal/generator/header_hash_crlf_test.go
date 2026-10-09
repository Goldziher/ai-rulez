package generator

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// injectHashesIn keeps one line ending: content that uses CRLF gets CRLF hash
// lines, and LF content stays LF. A lone LF in CRLF content would make a mixed file.
func TestInjectHashesKeepsTheLineEnding(t *testing.T) {
	tests := []struct {
		name, path, in string
	}{
		{"frontmatter", "SKILL.md", "---\r\nname: s\r\ndescription: d\r\n---\r\n\r\nBody\r\n"},
		{"empty frontmatter", "SKILL.md", "---\r\n---\r\nBody\r\n"},
		{"html banner", "CLAUDE.md", "<!--\r\nGenerated\r\n-->\r\n\r\nBody\r\n"},
		{"line comments", "settings.yaml", "# Generated\r\n\r\nkey: value\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := injectHashesIn(nil, tt.in, tt.path, "abc123", "def456")

			assert.Contains(t, out, "Content-Hash: abc123")
			assert.Contains(t, out, "Source-Hash: def456")
			assert.Equal(t, strings.Count(out, "\n"), strings.Count(out, "\r\n"), "every line break is CRLF: %q", out)
			// And it reads back.
			assert.Equal(t, "abc123", firstHash(out))
		})
	}
}

func TestInjectHashesLeavesLFContentAlone(t *testing.T) {
	out := injectHashesIn(nil, "---\nname: s\n---\nBody\n", "SKILL.md", "abc123", "")
	assert.NotContains(t, out, "\r")
	assert.Contains(t, out, "# Content-Hash: abc123\n---")
}

func firstHash(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if c, _ := hashFromLine(strings.TrimRight(line, "\r")); c != "" {
			return c
		}
	}
	return ""
}
