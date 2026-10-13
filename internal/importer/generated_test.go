package importer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsGeneratedRequiresTheMarkerAsTheHeader(t *testing.T) {
	const header = "<!--\n🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT\nProject: demo\n-->\n\n# demo\n"
	tests := []struct {
		name string
		file string
		data string
		want bool
	}{
		{"html header", "AGENTS.md", header, true},
		{"html header with CRLF", "AGENTS.md", strings.ReplaceAll(header, "\n", "\r\n"), true},
		{"marker only in the header comment", "AGENTS.md", "<!-- AI-RULEZ :: nothing else -->\n# demo\n", true},
		{"hash header", "rules.toml", "# 🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT\nname = 'x'\n", true},
		{"header after frontmatter", "rule.md", "---\ndescription: x\n---\n" + header, true},
		{"freshness line in frontmatter", "rule.md", "---\ndescription: x\n# Content-Hash: blake3:abc\n---\nBody\n", true},
		{"freshness line in frontmatter with CRLF", "rule.md", "---\r\ndescription: x\r\n# Content-Hash: blake3:abc\r\n---\r\nBody\r\n", true},
		{"marker quoted in prose", "AGENTS.md", "# demo\n\nGenerated files start with `AI-RULEZ ::` in a comment.\n", false},
		{"marker in a code block under a heading", "AGENTS.md", "# demo\n\n```\n<!--\nAI-RULEZ :: GENERATED FILE\n-->\n```\n", false},
		{"freshness line in the body", "rule.md", "---\ndescription: x\n---\n# Content-Hash: blake3:abc\n", false},
		{"plain file", "AGENTS.md", "# demo\n\nUse tabs.\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := IsGenerated(tt.file, []byte(tt.data))

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
