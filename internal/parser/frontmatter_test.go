package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseFrontmatter_NoFrontmatter tests content without frontmatter
func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := "# Hello World\n\nThis is content without frontmatter"

	metadata, actualContent, err := ParseFrontmatter(content)

	require.NoError(t, err)
	assert.Nil(t, metadata)
	assert.Equal(t, content, actualContent)
}

// TestParseFrontmatter_ValidFrontmatter tests parsing valid frontmatter
func TestParseFrontmatter_ValidFrontmatter(t *testing.T) {
	content := `---
priority: high
targets:
  - go
  - typescript
---

# Content
This is the actual content.`

	metadata, actualContent, err := ParseFrontmatter(content)

	require.NoError(t, err)
	require.NotNil(t, metadata)
	assert.Equal(t, "high", metadata.Priority)
	assert.Equal(t, []string{"go", "typescript"}, metadata.Targets)
	assert.Equal(t, "# Content\nThis is the actual content.", actualContent)
}

// TestParseFrontmatter_EmptyFrontmatter tests parsing with empty frontmatter
func TestParseFrontmatter_EmptyFrontmatter(t *testing.T) {
	content := `---
---

Content here`

	metadata, actualContent, err := ParseFrontmatter(content)

	require.NoError(t, err)
	require.NotNil(t, metadata)
	assert.Equal(t, "Content here", actualContent)
}

// TestParseFrontmatter_NoClosingMarker tests content with opening --- but no closing
func TestParseFrontmatter_NoClosingMarker(t *testing.T) {
	content := `---
priority: high

This is not frontmatter without closing ---`

	metadata, actualContent, err := ParseFrontmatter(content)

	require.NoError(t, err)
	assert.Nil(t, metadata)
	assert.Equal(t, content, actualContent)
}

// TestParseFrontmatter_TooShort tests content too short to be frontmatter
func TestParseFrontmatter_TooShort(t *testing.T) {
	content := `---
---`

	metadata, actualContent, err := ParseFrontmatter(content)

	require.NoError(t, err)
	assert.Nil(t, metadata)
	assert.Equal(t, content, actualContent)
}

// TestParseFrontmatter_MalformedYAML tests parsing with malformed YAML (Issue #4)
func TestParseFrontmatter_MalformedYAML(t *testing.T) {
	content := `---
priority: high
targets:
  - go
  invalid yaml here
---

Content`

	metadata, contentResult, err := ParseFrontmatter(content)

	// Should return error instead of silently ignoring (Issue #4 fix)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse frontmatter YAML")
	// Should still return original content as fallback
	assert.Equal(t, content, contentResult)
	assert.Nil(t, metadata)
}

// TestParseFrontmatter_WindowsCRLF tests Windows CRLF line endings (Issue #8)
func TestParseFrontmatter_WindowsCRLF(t *testing.T) {
	// Windows uses CRLF (\r\n) for line endings
	content := "---\r\npriority: high\r\ntargets:\r\n  - go\r\n---\r\n\r\nContent here"

	metadata, actualContent, err := ParseFrontmatter(content)

	require.NoError(t, err)
	require.NotNil(t, metadata)
	assert.Equal(t, "high", metadata.Priority)
	assert.Equal(t, []string{"go"}, metadata.Targets)
	// Content should have normalized line endings
	assert.Equal(t, "Content here", actualContent)
}

// TestParseFrontmatter_MixedLineEndings tests mixed CRLF and LF
func TestParseFrontmatter_MixedLineEndings(t *testing.T) {
	// Mix of CRLF and LF
	content := "---\r\npriority: medium\r\ntargets:\n  - python\r\n---\n\nMixed endings content"

	metadata, contentResult, err := ParseFrontmatter(content)

	require.NoError(t, err)
	require.NotNil(t, metadata)
	assert.Equal(t, "medium", metadata.Priority)
	assert.Equal(t, []string{"python"}, metadata.Targets)
	assert.Equal(t, "Mixed endings content", contentResult)
}

// TestParseFrontmatter_MacLineEndings tests old Mac CR line endings
func TestParseFrontmatter_MacLineEndings(t *testing.T) {
	// Old Mac used CR (\r) only for line endings
	content := "---\rpriority: low\rtargets:\r  - rust\r---\r\rMac line ending content"

	metadata, _, err := ParseFrontmatter(content)

	require.NoError(t, err)
	require.NotNil(t, metadata)
	assert.Equal(t, "low", metadata.Priority)
	assert.Contains(t, metadata.Targets, "rust")
}

// TestParseFrontmatterNonFatal tests non-fatal parsing
func TestParseFrontmatterNonFatal(t *testing.T) {
	t.Run("returns nil on parse error", func(t *testing.T) {
		// Malformed YAML
		content := `---
priority: high
invalid: [syntax
---

Content`

		metadata, actualContent := ParseFrontmatterNonFatal(content)

		// Non-fatal version should return nil metadata on error
		assert.Nil(t, metadata)
		// Should return original content
		assert.Equal(t, content, actualContent)
	})

	t.Run("returns parsed metadata on success", func(t *testing.T) {
		content := `---
priority: high
---

Content`

		metadata, actualContent := ParseFrontmatterNonFatal(content)

		assert.NotNil(t, metadata)
		assert.Equal(t, "high", metadata.Priority)
		assert.Equal(t, "Content", actualContent)
	})
}

// TestNormalizeLFLineEndings tests line ending normalization
func TestNormalizeLFLineEndings(t *testing.T) {
	t.Run("normalizes CRLF to LF", func(t *testing.T) {
		content := "line1\r\nline2\r\nline3"
		normalized := normalizeLFLineEndings(content)
		assert.Equal(t, "line1\nline2\nline3", normalized)
	})

	t.Run("handles CR only", func(t *testing.T) {
		content := "line1\rline2\rline3"
		normalized := normalizeLFLineEndings(content)
		assert.Equal(t, "line1\nline2\nline3", normalized)
	})

	t.Run("preserves LF", func(t *testing.T) {
		content := "line1\nline2\nline3"
		normalized := normalizeLFLineEndings(content)
		assert.Equal(t, content, normalized)
	})

	t.Run("handles mixed line endings", func(t *testing.T) {
		content := "line1\r\nline2\rline3\nline4"
		normalized := normalizeLFLineEndings(content)
		assert.Equal(t, "line1\nline2\nline3\nline4", normalized)
	})
}

// TestParseFrontmatter_ComplexYAML tests parsing complex YAML frontmatter
func TestParseFrontmatter_ComplexYAML(t *testing.T) {
	content := `---
priority: critical
targets:
  - go
  - typescript
  - python
extra:
  some_key: some_value
---

# Header

Content with multiple lines
and paragraphs`

	metadata, actualContent, err := ParseFrontmatter(content)

	require.NoError(t, err)
	require.NotNil(t, metadata)
	assert.Equal(t, "critical", metadata.Priority)
	assert.Equal(t, []string{"go", "typescript", "python"}, metadata.Targets)
	assert.Contains(t, actualContent, "# Header")
	assert.Contains(t, actualContent, "Content with multiple lines")
}

// TestParseFrontmatter_FrontmatterWithBlankLines tests frontmatter with blank lines
func TestParseFrontmatter_FrontmatterWithBlankLines(t *testing.T) {
	content := `---
priority: high

targets:
  - go
---

Content`

	metadata, _, err := ParseFrontmatter(content)

	// Blank lines in YAML are valid
	require.NoError(t, err)
	require.NotNil(t, metadata)
	assert.Equal(t, "high", metadata.Priority)
}

// TestParseFrontmatter_WhitespaceHandling tests whitespace in frontmatter
func TestParseFrontmatter_WhitespaceHandling(t *testing.T) {
	content := `---
priority:    high
targets:
  -   go
  -   typescript
---

Content`

	metadata, _, err := ParseFrontmatter(content)

	require.NoError(t, err)
	require.NotNil(t, metadata)
	// YAML unmarshaling should handle whitespace
	assert.NotEmpty(t, metadata.Priority)
	assert.NotEmpty(t, metadata.Targets)
}

// TestTruncateString tests the truncateString helper
func TestTruncateString(t *testing.T) {
	t.Run("truncates long string", func(t *testing.T) {
		result := truncateString("This is a very long string that should be truncated", 20)
		assert.Equal(t, "This is a very long ...", result)
	})

	t.Run("doesn't truncate short string", func(t *testing.T) {
		result := truncateString("short", 20)
		assert.Equal(t, "short", result)
	})

	t.Run("handles empty string", func(t *testing.T) {
		result := truncateString("", 20)
		assert.Equal(t, "", result)
	})
}

// TestParseFrontmatter_IntegrationWithMetadata tests integration with MetadataV3
func TestParseFrontmatter_IntegrationWithMetadata(t *testing.T) {
	content := `---
priority: high
targets:
  - go
---

# Rule Content

This is a rule about Go practices.`

	metadata, actualContent, err := ParseFrontmatter(content)

	require.NoError(t, err)
	require.NotNil(t, metadata)

	// Verify metadata is properly typed as MetadataV3
	var _ = metadata
	assert.Equal(t, "high", metadata.Priority)
	assert.Contains(t, actualContent, "# Rule Content")
}

// TestParseFrontmatter_EdgeCaseEmpty tests empty content
func TestParseFrontmatter_EdgeCaseEmpty(t *testing.T) {
	metadata, actualContent, err := ParseFrontmatter("")

	require.NoError(t, err)
	assert.Nil(t, metadata)
	assert.Equal(t, "", actualContent)
}

// TestParseFrontmatter_EdgeCaseOnlyFrontmatter tests content with only frontmatter
func TestParseFrontmatter_EdgeCaseOnlyFrontmatter(t *testing.T) {
	content := `---
priority: high
---`

	metadata, actualContent, err := ParseFrontmatter(content)

	require.NoError(t, err)
	require.NotNil(t, metadata)
	assert.Equal(t, "high", metadata.Priority)
	assert.Equal(t, "", actualContent)
}

// TestScanLinesPreservingEmpty tests line scanning
func TestScanLinesPreservingEmpty(t *testing.T) {
	content := "line1\n\nline3"
	lines := scanLinesPreservingEmpty(content)

	assert.Equal(t, 3, len(lines))
	assert.Equal(t, "line1", lines[0])
	assert.Equal(t, "", lines[1])
	assert.Equal(t, "line3", lines[2])
}
