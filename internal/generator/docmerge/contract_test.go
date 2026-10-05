package docmerge_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/generator/docmerge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var allFormats = []docmerge.Format{
	docmerge.FormatJSON, docmerge.FormatJSONC, docmerge.FormatTOML, docmerge.FormatYAML,
}

func TestApply_RemoveOnlyOnMissingFileHasEmptyBody(t *testing.T) {
	owned := []docmerge.OwnedKey{{Name: "servers", Remove: true}}
	for _, format := range allFormats {
		t.Run(string(format), func(t *testing.T) {
			missing, err := docmerge.Apply(filepath.Join(t.TempDir(), "missing"), format, owned)
			require.NoError(t, err)
			assert.Empty(t, missing.Body, "nothing to write: the caller must not create a file")
			assert.Empty(t, missing.Claims)

			fromText, err := docmerge.ApplyDocument("x", format, "  \n", owned)
			require.NoError(t, err)
			assert.Empty(t, fromText.Body)
		})
	}
}

func TestApply_MembersAndElementsTogetherIsAnError(t *testing.T) {
	owned := []docmerge.OwnedKey{{
		Name: "x", Value: map[string]any{"a": 1}, Members: true, Elements: []any{1},
	}}
	for _, format := range allFormats {
		t.Run(string(format), func(t *testing.T) {
			_, err := docmerge.Apply(filepath.Join(t.TempDir(), "missing"), format, owned)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "Members and Elements")

			_, err = docmerge.ApplyDocument("x", format, "# c\n", owned)
			require.Error(t, err)
		})
	}
}

func TestApplyDocument_MatchesApply(t *testing.T) {
	owned := []docmerge.OwnedKey{{Name: "servers", Value: map[string]any{"gen": map[string]any{"command": "x"}}, Members: true}}
	docs := map[docmerge.Format]string{
		docmerge.FormatJSON:  "{\n  \"a\": 1\n}\n",
		docmerge.FormatJSONC: "{\n  // c\n  \"a\": 1\n}\n",
		docmerge.FormatTOML:  "a = 1\n",
		docmerge.FormatYAML:  "a: 1\n",
	}
	for format, doc := range docs {
		t.Run(string(format), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "doc")
			require.NoError(t, writeFile(path, doc))

			fromFile, err := docmerge.Apply(path, format, owned)
			require.NoError(t, err)
			fromText, err := docmerge.ApplyDocument(path, format, doc, owned)
			require.NoError(t, err)

			assert.Equal(t, fromFile, fromText)
		})
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
