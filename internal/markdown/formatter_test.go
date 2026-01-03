package markdown

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatter_Format(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		validate func(t *testing.T, output string)
	}{
		{
			name: "simple heading and paragraph",
			input: `# Hello World

This is a paragraph.`,
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "# Hello World")
				assert.Contains(t, output, "This is a paragraph.")
				assert.True(t, strings.HasSuffix(output, "\n"))
			},
		},
		{
			name: "multiple headings",
			input: `# Main Title

## Section 1

Content here.

### Subsection

More content.`,
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "# Main Title")
				assert.Contains(t, output, "## Section 1")
				assert.Contains(t, output, "### Subsection")
			},
		},
		{
			name: "list formatting",
			input: `# List Example

- Item 1
- Item 2
  - Nested item
- Item 3`,
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "- Item 1")
				assert.Contains(t, output, "- Item 2")
				assert.Contains(t, output, "- Item 3")
			},
		},
		{
			name:  "code blocks",
			input: "# Code Example\n\n```go\nfunc main() {\n\tprintln(\"hello\")\n}\n```",
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "```go")
				assert.Contains(t, output, "func main()")
			},
		},
		{
			name: "emphasis and strong",
			input: `# Text Formatting

This is *italic* and this is **bold** text.`,
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "*italic*")
				assert.Contains(t, output, "**bold**")
			},
		},
		{
			name: "links and images",
			input: `# Links

[Link text](https://example.com)

![Alt text](image.png)`,
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "[Link text](https://example.com)")
				assert.Contains(t, output, "![Alt text](image.png)")
			},
		},
		{
			name: "trailing newline handling",
			input: `# Title

Content without trailing newline`,
			validate: func(t *testing.T, output string) {
				assert.True(t, strings.HasSuffix(output, "\n"), "output should end with newline")
				// Should have exactly one trailing newline
				assert.False(t, strings.HasSuffix(output, "\n\n"), "output should not have multiple trailing newlines")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewFormatter()
			output, err := formatter.Format(tt.input)
			require.NoError(t, err)
			tt.validate(t, output)
		})
	}
}

func TestFormatString(t *testing.T) {
	input := `# Test

This is a test.`

	output, err := FormatString(input)
	require.NoError(t, err)
	assert.Contains(t, output, "# Test")
	assert.Contains(t, output, "This is a test.")
	assert.True(t, strings.HasSuffix(output, "\n"))
}

func TestFormatter_EmptyInput(t *testing.T) {
	formatter := NewFormatter()
	output, err := formatter.Format("")
	require.NoError(t, err)
	assert.Equal(t, "\n", output)
}

func TestFormatter_ComplexMarkdown(t *testing.T) {
	input := `# AI-Rulez Documentation

## Overview

AI-Rulez is a **powerful** tool for managing AI configurations.

### Features

- Configuration management
- Multi-preset support
  - Claude
  - Cursor
  - Gemini
- Domain-based organization

### Code Example

` + "```yaml" + `
version: "3.0"
name: my-project
presets:
  - claude
` + "```" + `

For more information, visit [our documentation](https://github.com/Goldziher/ai-rulez).`

	formatter := NewFormatter()
	output, err := formatter.Format(input)
	require.NoError(t, err)

	// Verify key elements are preserved
	assert.Contains(t, output, "# AI-Rulez Documentation")
	assert.Contains(t, output, "## Overview")
	assert.Contains(t, output, "### Features")
	assert.Contains(t, output, "- Configuration management")
	assert.Contains(t, output, "```yaml")
	assert.Contains(t, output, "[our documentation](https://github.com/Goldziher/ai-rulez)")
	assert.True(t, strings.HasSuffix(output, "\n"))
}
