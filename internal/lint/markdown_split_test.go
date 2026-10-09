package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// parseDoc locates the frontmatter by the rules of internal/frontmatter.
func TestParseDocLocatesTheFrontmatterByTheSharedRules(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int
	}{
		{"plain", "---\na: 1\n---\nBody\n", 3},
		{"crlf", "---\r\na: 1\r\n---\r\nBody\r\n", 3},
		{"BOM before the opening fence", "\xef\xbb\xbf---\na: 1\n---\nBody\n", 3},
		{"empty block", "---\n---\nBody\n", 2},
		{"trailing spaces on the fences", "--- \na: 1\n---\t\nBody\n", 3},
		{"an indented fence neither opens nor closes", " ---\na: 1\n---\nBody\n", 0},
		{"an indented line does not close", "---\na: 1\n ---\nb: 2\n---\nBody\n", 5},
		{"unclosed", "---\na: 1\nBody\n", 0},
		{"none", "# Title\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseDoc(tt.raw).bodyStart)
		})
	}
}
