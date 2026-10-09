package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The loader finds the end of a frontmatter block by the rules of
// internal/frontmatter. These cases pin the inputs on which that differs from
// the splitter the loader used to carry.
func TestParseFrontmatterFollowsTheSharedSplitRules(t *testing.T) {
	t.Run("a UTF-8 BOM before the opening fence is ignored", func(t *testing.T) {
		meta, body, malformed := parseFrontmatter("\xef\xbb\xbf---\nusage: Use it\n---\nBody\n")

		require.NotNil(t, meta)
		assert.False(t, malformed)
		assert.Equal(t, "Use it", meta.Usage)
		assert.Equal(t, "Body\n", body)
	})
	t.Run("an empty block is a block", func(t *testing.T) {
		meta, body, malformed := parseFrontmatter("---\n---")

		require.NotNil(t, meta)
		assert.False(t, malformed)
		assert.Empty(t, body)
	})
	t.Run("an indented dash line does not close the block", func(t *testing.T) {
		meta, body, _ := parseFrontmatter("---\nusage: a\n ---\nstill yaml\n---\nBody\n")

		assert.Nil(t, meta, "the block runs to the real fence and holds invalid YAML or no usage")
		assert.Equal(t, "Body\n", body)
		assert.False(t, hasUnclosedFrontmatter("---\nusage: a\n ---\nstill yaml\n---\nBody\n"))
	})
	t.Run("trailing spaces on a fence are ignored", func(t *testing.T) {
		meta, body, _ := parseFrontmatter("--- \nusage: a\n---  \nBody\n")

		require.NotNil(t, meta)
		assert.Equal(t, "a", meta.Usage)
		assert.Equal(t, "Body\n", body)
	})
	t.Run("the blank line after a CRLF block is dropped from the body like an LF one", func(t *testing.T) {
		_, body, _ := parseFrontmatter("---\r\nusage: a\r\n---\r\n\r\nBody\r\n")

		assert.Equal(t, "Body\r\n", body)
	})
	t.Run("a block that never closes stays plain content", func(t *testing.T) {
		meta, body, malformed := parseFrontmatter("---\nusage: a\nBody\n")

		assert.Nil(t, meta)
		assert.False(t, malformed)
		assert.Equal(t, "---\nusage: a\nBody\n", body)
		assert.True(t, hasUnclosedFrontmatter("---\nusage: a\nBody\n"))
	})
}

func TestIsOKFListingReadsTheFrontmatterByTheSharedRules(t *testing.T) {
	assert.True(t, IsOKFListing([]byte("---\r\nokf_version: 1\r\n---\r\n* [A](a.md)\r\n")), "CRLF")
	assert.True(t, IsOKFListing([]byte("---\n---\n- [A](a.md)\n")), "an empty block")
	assert.False(t, IsOKFListing([]byte("---\nokf_version: 1\n----\n- [A](a.md)\n")), "a longer dash run is not a fence")
	assert.False(t, IsOKFListing([]byte("---\nokf_version: 1\n- [A](a.md)\n")), "unclosed")
}
