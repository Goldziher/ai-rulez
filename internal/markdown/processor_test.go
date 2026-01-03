package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripFirstHeading(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "strips first H1",
			input: `# Main Title

Content here.

## Section

More content.`,
			expected: `Content here.

## Section

More content.`,
		},
		{
			name: "strips H1 with blank lines after",
			input: `# Title


Content starts here.`,
			expected: `Content starts here.`,
		},
		{
			name: "preserves H2 and below",
			input: `## Not H1

Content.

### Subsection`,
			expected: `## Not H1

Content.

### Subsection`,
		},
		{
			name: "handles content with no H1",
			input: `Some content without heading.

## Section`,
			expected: `Some content without heading.

## Section`,
		},
		{
			name: "handles empty content after H1",
			input: `# First Title

`,
			expected: ``,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripFirstHeading(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDemoteHeadings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		levels   int
		expected string
	}{
		{
			name: "demote by 1 level",
			input: `# H1
## H2
### H3`,
			levels: 1,
			expected: `## H1
### H2
#### H3`,
		},
		{
			name: "demote by 2 levels",
			input: `# H1
## H2
### H3`,
			levels: 2,
			expected: `### H1
#### H2
##### H3`,
		},
		{
			name: "respects max heading level",
			input: `#### H4
##### H5
###### H6`,
			levels: 2,
			expected: `###### H4
###### H5
###### H6`,
		},
		{
			name: "preserves non-heading lines",
			input: `# Title

Some content here.

## Section

More content.`,
			levels: 1,
			expected: `## Title

Some content here.

### Section

More content.`,
		},
		{
			name: "no change with levels=0",
			input: `# H1
## H2`,
			levels: 0,
			expected: `# H1
## H2`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DemoteHeadings(tt.input, tt.levels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeBlankLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "reduces 3 blank lines to 2",
			input:    "Line 1\n\n\n\nLine 2",
			expected: "Line 1\n\nLine 2",
		},
		{
			name:     "reduces 5 blank lines to 2",
			input:    "Line 1\n\n\n\n\n\nLine 2",
			expected: "Line 1\n\nLine 2",
		},
		{
			name:     "preserves 2 blank lines",
			input:    "Line 1\n\n\nLine 2",
			expected: "Line 1\n\nLine 2",
		},
		{
			name:     "preserves 1 blank line",
			input:    "Line 1\n\nLine 2",
			expected: "Line 1\n\nLine 2",
		},
		{
			name:     "handles multiple occurrences",
			input:    "A\n\n\n\nB\n\n\n\nC",
			expected: "A\n\nB\n\nC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeBlankLines(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessEmbeddedContent(t *testing.T) {
	input := `# Rule Title

This is the rule content.


It has multiple blank lines.

## Subsection

More content.


`

	expected := `This is the rule content.

It has multiple blank lines.

## Subsection

More content.`

	result := ProcessEmbeddedContent(input)
	assert.Equal(t, expected, result)
}

func TestProcessEmbeddedContent_RealWorld(t *testing.T) {
	// Simulate content from a rule file
	input := `# Cross Runtime Distribution

- Keep Go, npm, and PyPI entry points aligned when you add or rename capabilities.
- Update documentation in ` + "`docs/`, `release/`, and `README.md`" + ` when CLI surface changes.
- Synchronize version bumps across ` + "`cmd/commands/root.go`, `package.json`, `pyproject.toml`" + `, and release metadata.


- Maintain schema updates in ` + "`schema/`" + ` and refresh generator/enforcer fixtures in ` + "`tests/`" + ` when behavior changes.
`

	result := ProcessEmbeddedContent(input)

	// Should not contain the H1
	assert.NotContains(t, result, "# Cross Runtime Distribution")

	// Should start with the content
	assert.Contains(t, result, "- Keep Go, npm")

	// Should normalize blank lines (no triple newlines)
	assert.NotContains(t, result, "\n\n\n\n")
}
