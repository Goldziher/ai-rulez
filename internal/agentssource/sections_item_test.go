package agentssource

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadItemReturnsTheSectionAsWritten(t *testing.T) {
	tests := []struct {
		name string
		text string
		item string
		want string
	}{
		{"LF", sectionsText, "agents-setup", "## Setup\n\nRun it.\n\n"},
		{"CRLF", "# Demo\r\n\r\n## Setup\r\n\r\nRun it.\r\n\r\n## Style\r\n\r\nTabs.\r\n", "agents-setup", "## Setup\r\n\r\nRun it.\r\n\r\n"},
		{"BOM", "\ufeff## Setup\n\nRun it.\n\n## Style\n\nTabs.\n", "agents-style", "## Style\n\nTabs.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base, _ := writeAgentsMD(t, tt.text, 0o644)

			// Act
			got, err := ReadItem(base, tt.item, true, nil)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReadItemWithoutSplitIsTheWholeFileAndUnknownNamesFail(t *testing.T) {
	// Arrange
	base, _ := writeAgentsMD(t, sectionsText, 0o644)

	// Act
	whole, wholeErr := ReadItem(base, "agents", false, nil)
	_, missingErr := ReadItem(base, "agents-nope", true, nil)

	// Assert
	require.NoError(t, wholeErr)
	assert.Equal(t, sectionsText, whole)
	require.Error(t, missingErr)
	assert.Contains(t, missingErr.Error(), "no item")
}

func TestUpdateItemKeepsTheBytesAroundTheSection(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		content string
		want    string
	}{
		{"LF", sectionsText, "## Setup\n\nRun it twice.\n\n", "# Demo\n\nIntro.\n\n## Setup\n\nRun it twice.\n\n## Style\n\nTabs.\n"},
		{"CRLF file with LF content", "# Demo\r\n\r\n## Setup\r\n\r\nRun it.\r\n\r\n## Style\r\n\r\nTabs.\r\n", "## Setup\n\nTwice.\n\n",
			"# Demo\r\n\r\n## Setup\r\n\r\nTwice.\r\n\r\n## Style\r\n\r\nTabs.\r\n"},
		{"BOM", "\ufeff## Setup\n\nRun it.\n\n## Style\n\nTabs.\n", "## Setup\n\nTwice.\n\n", "\ufeff## Setup\n\nTwice.\n\n## Style\n\nTabs.\n"},
		{"heading text inside a code fence is not a section", sectionsText, "## Setup\n\n```sh\n## comment\n```\n\n",
			"# Demo\n\nIntro.\n\n## Setup\n\n```sh\n## comment\n```\n\n## Style\n\nTabs.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base, path := writeAgentsMD(t, tt.text, 0o600)

			// Act
			err := UpdateItem(base, "agents-setup", tt.content, true, nil)

			// Assert
			require.NoError(t, err)
			got, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, tt.want, string(got))
			info, statErr := os.Stat(path)
			require.NoError(t, statErr)
			if runtime.GOOS != "windows" { // Windows has no POSIX permission bits
				assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
			}
		})
	}
}

func TestUpdateItemOnTheLastSectionKeepsTheFinalLineEnding(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		content string
		want    string
	}{
		{"LF with trailing newline", "## Setup\n\nRun it.\n\n## Style\n\nTabs.\n", "## Style\n\nSpaces.\n",
			"## Setup\n\nRun it.\n\n## Style\n\nSpaces.\n"},
		{"LF with trailing newline and content without one", "## Setup\n\nRun it.\n\n## Style\n\nTabs.\n", "## Style\n\nSpaces.",
			"## Setup\n\nRun it.\n\n## Style\n\nSpaces.\n"},
		{"LF without trailing newline", "## Setup\n\nRun it.\n\n## Style\n\nTabs.", "## Style\n\nSpaces.",
			"## Setup\n\nRun it.\n\n## Style\n\nSpaces."},
		{"LF without trailing newline and content with one", "## Setup\n\nRun it.\n\n## Style\n\nTabs.", "## Style\n\nSpaces.\n",
			"## Setup\n\nRun it.\n\n## Style\n\nSpaces."},
		{"CRLF with trailing newline", "## Setup\r\n\r\nRun it.\r\n\r\n## Style\r\n\r\nTabs.\r\n", "## Style\n\nSpaces.\n",
			"## Setup\r\n\r\nRun it.\r\n\r\n## Style\r\n\r\nSpaces.\r\n"},
		{"CRLF without trailing newline", "## Setup\r\n\r\nRun it.\r\n\r\n## Style\r\n\r\nTabs.", "## Style\r\n\r\nSpaces.",
			"## Setup\r\n\r\nRun it.\r\n\r\n## Style\r\n\r\nSpaces."},
		{"CRLF without trailing newline and LF content", "## Setup\r\n\r\nRun it.\r\n\r\n## Style\r\n\r\nTabs.", "## Style\n\nSpaces.\n",
			"## Setup\r\n\r\nRun it.\r\n\r\n## Style\r\n\r\nSpaces."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base, path := writeAgentsMD(t, tt.text, 0o644)

			// Act
			err := UpdateItem(base, "agents-style", tt.content, true, nil)

			// Assert
			require.NoError(t, err)
			got, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestUpdateItemRefusesWhatWouldChangeTheSections(t *testing.T) {
	tests := []struct {
		name    string
		item    string
		content string
	}{
		{"drops the heading", "agents-setup", "Run it.\n"},
		{"renames the heading", "agents-setup", "## Other\n\nRun it.\n"},
		{"adds a section", "agents-setup", "## Setup\n\nA.\n\n## More\n\nB.\n"},
		{"preamble gaining a heading", "agents", "# Demo\n\n## Intro\n\nx\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base, path := writeAgentsMD(t, sectionsText, 0o644)

			// Act
			err := UpdateItem(base, tt.item, tt.content, true, nil)

			// Assert
			require.Error(t, err)
			got, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, sectionsText, string(got))
		})
	}
}

func TestUpdateItemDetectsAConcurrentEdit(t *testing.T) {
	// Arrange
	base, path := writeAgentsMD(t, sectionsText, 0o644)
	edited := sectionsText + "\nAdded by someone else.\n"

	// Act
	err := updateItem(base, "agents-setup", "## Setup\n\nNew.\n\n", true, nil, func() {
		require.NoError(t, os.WriteFile(path, []byte(edited), 0o644))
	})

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "changed while the section was being updated")
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, edited, string(got))
	entries, dirErr := os.ReadDir(base)
	require.NoError(t, dirErr)
	assert.Len(t, entries, 1, "no temporary file is left behind")
}

func TestUpdateItemRefusesTheWholeFileAndASymlink(t *testing.T) {
	// Arrange
	base, path := writeAgentsMD(t, sectionsText, 0o644)

	// Act
	err := UpdateItem(base, "agents", "# New\n", false, nil)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "one item")
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, sectionsText, string(got))
}
