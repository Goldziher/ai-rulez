package conformance

import (
	"bytes"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AGENTS.md (https://agents.md) is plain Markdown with no required fields and
// no schema. The checks cover what the format does ask for: a Markdown file
// named AGENTS.md at the project root that carries the project's guidance and
// no frontmatter a reader would take for content.
func TestGeneratedAgentsMDIsPlainMarkdownAtTheRoot(t *testing.T) {
	// Arrange
	dir := project(t, map[string]string{
		".ai-rulez/config.toml":   "version = \"5.0\"\nname = \"conf\"\npresets = [\"codex\"]\n",
		".ai-rulez/rules/test.md": "# Testing\n\nRun the unit tests before every commit.\n",
	})

	// Act
	run(t, dir, "generate", "--yes")

	// Assert
	data := read(t, dir, "AGENTS.md")
	assert.True(t, utf8.Valid(data))
	assert.False(t, bytes.HasPrefix(data, []byte("---")), "AGENTS.md must not start with frontmatter")
	assert.True(t, bytes.HasSuffix(data, []byte("\n")))
	require.Contains(t, string(data), "# conf")
	assert.Contains(t, string(data), "Run the unit tests before every commit.")
}
