package docmerge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	mdBegin = "<!-- ai-rulez:checks:begin -->"
	mdEnd   = "<!-- ai-rulez:checks:end -->"
)

func mdOwned(text string) []OwnedKey { return []OwnedKey{{Name: "checks", Value: text}} }

func TestMarkdown_CreateWholeFileWithHeader(t *testing.T) {
	// Arrange
	owned := []OwnedKey{{Name: HeaderKey, Value: "---\nname: guidelines\n---"}, {Name: "checks", Value: "## a\n\nbody\n"}}

	// Act
	res, err := ApplyDocument("SKILL.md", FormatMarkdown, "", owned)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "---\nname: guidelines\n---\n\n"+mdBegin+"\n## a\n\nbody\n"+mdEnd+"\n", res.Body)
	assert.False(t, res.PartiallyOwned)
	assert.Len(t, res.Claims, 2)

	// Unmerge of a document that holds nothing else deletes it.
	un, err := UnmergeDocument("SKILL.md", FormatMarkdown, res.Body, res.Claims)
	require.NoError(t, err)
	assert.True(t, un.Changed)
	assert.True(t, un.Empty)
}

func TestMarkdown_AppendsToAHandWrittenFileAndRestoresIt(t *testing.T) {
	tests := []struct {
		name     string
		existing string
	}{
		{name: "ends with a newline", existing: "# Review rules\n\nBe kind.\n"},
		{name: "no final newline", existing: "# Review rules\n\nBe kind."},
		{name: "crlf", existing: "# Review rules\r\n\r\nBe kind.\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			res, err := ApplyDocument("REVIEW.md", FormatMarkdown, tt.existing, mdOwned("## a\n\nbody\n"))
			require.NoError(t, err)

			// Assert: the user's text is untouched and the file counts as shared.
			assert.Contains(t, res.Body, tt.existing[:len(tt.existing)-len(tt.existing)/20]) // all but the tail
			assert.True(t, res.PartiallyOwned)
			assert.Contains(t, res.Body, mdBegin)

			un, err := UnmergeDocument("REVIEW.md", FormatMarkdown, res.Body, res.Claims)
			require.NoError(t, err)
			assert.False(t, un.Empty)
			assert.Equal(t, tt.existing, un.Body, "clean must give the hand-written file back byte for byte")
		})
	}
}

func TestMarkdown_ReplacesOnlyTheBlock(t *testing.T) {
	// Arrange
	existing := "intro\n\n" + mdBegin + "\nold\n" + mdEnd + "\n\noutro\n"

	// Act
	res, err := ApplyDocument("REVIEW.md", FormatMarkdown, existing, mdOwned("new"))
	require.NoError(t, err)
	again, err := ApplyDocument("REVIEW.md", FormatMarkdown, res.Body, mdOwned("new"))
	require.NoError(t, err)

	// Assert
	assert.Equal(t, "intro\n\n"+mdBegin+"\nnew\n"+mdEnd+"\n\noutro\n", res.Body)
	assert.Equal(t, res.Body, again.Body, "applying twice is stable")
	assert.True(t, res.PartiallyOwned)

	un, err := UnmergeDocument("REVIEW.md", FormatMarkdown, res.Body, res.Claims)
	require.NoError(t, err)
	assert.Equal(t, "intro\n\noutro\n", un.Body)
}

func TestMarkdown_UnmergeIgnoresAnEditInsideTheBlock(t *testing.T) {
	// Arrange: the markers delimit what is ours, whatever it holds now.
	res, err := ApplyDocument("R.md", FormatMarkdown, "keep\n", mdOwned("generated"))
	require.NoError(t, err)
	edited := "keep\n\n" + mdBegin + "\nhand edit\n" + mdEnd + "\n"

	// Act
	un, err := UnmergeDocument("R.md", FormatMarkdown, edited, res.Claims)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "keep\n", un.Body)
}

func TestMarkdown_RefusesUnpairedOrDuplicatedMarkers(t *testing.T) {
	tests := map[string]string{
		"begin only": mdBegin + "\ntext\n",
		"end only":   "text\n" + mdEnd + "\n",
		"two begins": mdBegin + "\na\n" + mdEnd + "\n" + mdBegin + "\nb\n",
		"end first":  mdEnd + "\n" + mdBegin + "\n",
	}
	for name, existing := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ApplyDocument("R.md", FormatMarkdown, existing, mdOwned("x"))
			require.Error(t, err)
			_, err = UnmergeDocument("R.md", FormatMarkdown, existing, []Claim{{Path: []string{"checks"}}})
			require.Error(t, err)
		})
	}
}

func TestMarkdown_TextMayNotContainItsOwnMarker(t *testing.T) {
	_, err := ApplyDocument("R.md", FormatMarkdown, "", mdOwned("a\n"+mdEnd+"\nb"))
	require.Error(t, err)
}

func TestMarkdown_IsAValidFormatButNotInferredFromTheExtension(t *testing.T) {
	_, ok := FormatFromPath("README.md")
	assert.False(t, ok, "an arbitrary .md file is not a merge target")
	assert.True(t, FormatMarkdown.Valid())
}
