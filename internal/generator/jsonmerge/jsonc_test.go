package jsonmerge_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serverOwned() []jsonmerge.OwnedKey {
	return []jsonmerge.OwnedKey{{Name: "mcpServers", Value: map[string]any{"gen": map[string]any{"command": "x"}}, Members: true}}
}

func TestApply_JSONC(t *testing.T) {
	tests := []struct {
		name  string
		doc   string
		owned []jsonmerge.OwnedKey
		want  string
	}{
		{
			name: "appends a new owned key and keeps comments",
			doc: `{
  // editor
  "editor.fontSize": 14, // px
  /* block */ "files.autoSave": "off"
}
`,
			owned: []jsonmerge.OwnedKey{{Name: "mcp", Value: map[string]any{"servers": map[string]any{}}}},
			want: `{
  // editor
  "editor.fontSize": 14, // px
  /* block */ "files.autoSave": "off",
  "mcp": {
    "servers": {}
  }
}
`,
		},
		{
			name: "keeps a trailing comma when appending",
			doc: `{
  "a": 1,
}
`,
			owned: []jsonmerge.OwnedKey{{Name: "b", Value: 2}},
			want: `{
  "a": 1,
  "b": 2,
}
`,
		},
		{
			name: "same-line comment after the last member stays with it",
			doc: `{
  "a": 1 // note
}
`,
			owned: []jsonmerge.OwnedKey{{Name: "b", Value: 2}},
			want: `{
  "a": 1, // note
  "b": 2
}
`,
		},
		{
			name: "replaces an owned value in place and keeps comments around it",
			doc: `{
  // servers
  "mcpServers": /* v */ {"old": {"command": "y"}}, // trailing
  "z": true
}
`,
			owned: []jsonmerge.OwnedKey{{Name: "mcpServers", Value: map[string]any{"gen": 1}}},
			want: `{
  // servers
  "mcpServers": /* v */ {
    "gen": 1
  }, // trailing
  "z": true
}
`,
		},
		{
			name: "merges members one by one",
			doc: `{
  "mcpServers": {
    // mine
    "mine": {"command": "mine"}, // keep
    "gen": {"command": "stale"},
  },
}
`,
			owned: serverOwned(),
			want: `{
  "mcpServers": {
    // mine
    "mine": {"command": "mine"}, // keep
    "gen": {
      "command": "x"
    },
  },
}
`,
		},
		{
			name:  "inline object stays inline",
			doc:   `{"a": 1, /* c */ "b": 2}`,
			owned: []jsonmerge.OwnedKey{{Name: "c", Value: []int{1}}},
			want:  `{"a": 1, /* c */ "b": 2, "c": [1]}`,
		},
		{
			name:  "empty object with a comment",
			doc:   "// header\n{\n  // nothing yet\n}\n",
			owned: []jsonmerge.OwnedKey{{Name: "a", Value: 1}},
			want:  "// header\n{\n  // nothing yet\n  \"a\": 1\n}\n",
		},
		{
			name:  "nested path under a commented ancestor",
			doc:   "{\n  \"mcp\": {\n    // timeout\n    \"timeout\": 5\n  }\n}\n",
			owned: []jsonmerge.OwnedKey{{Path: []string{"mcp", "servers"}, Value: map[string]any{"a": 1}}},
			want:  "{\n  \"mcp\": {\n    // timeout\n    \"timeout\": 5,\n    \"servers\": {\n      \"a\": 1\n    }\n  }\n}\n",
		},
		{
			name:  "remove drops the member and an emptied ancestor",
			doc:   "{\n  // keep me\n  \"a\": 1,\n  \"mcp\": {\n    \"servers\": {}\n  }\n}\n",
			owned: []jsonmerge.OwnedKey{{Path: []string{"mcp", "servers"}, Remove: true}},
			want:  "{\n  // keep me\n  \"a\": 1\n}\n",
		},
		{
			name:  "four-space indent and CRLF are matched",
			doc:   "{\r\n    // c\r\n    \"a\": 1\r\n}\r\n",
			owned: []jsonmerge.OwnedKey{{Name: "b", Value: map[string]any{"x": 1}}},
			want:  "{\r\n    // c\r\n    \"a\": 1,\r\n    \"b\": {\r\n        \"x\": 1\r\n    }\r\n}\r\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := writeFixture(t, "settings.jsonc", tt.doc)

			// Act
			result, err := jsonmerge.Apply(path, tt.owned)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.Body)
			assert.True(t, result.PartiallyOwned)

			// Idempotent: applying again changes nothing.
			again := writeFixture(t, "again.jsonc", result.Body)
			second, err := jsonmerge.Apply(again, tt.owned)
			require.NoError(t, err)
			assert.Equal(t, result.Body, second.Body)
		})
	}
}

func TestApply_JSONCRefusals(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{"array root", "// c\n[1, 2]\n"},
		{"trailing content", "{\"a\": 1 /* c */}\n{\"b\": 2}\n"},
		{"owned key is not an object", "{ // c\n \"mcpServers\": 3 }"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := writeFixture(t, "settings.jsonc", tt.doc)

			// Act
			_, err := jsonmerge.Apply(path, serverOwned())

			// Assert
			require.Error(t, err)
			onDisk, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, tt.doc, string(onDisk))
		})
	}
}

func TestApply_JSONCOwnedOnlyDocumentIsNotPartiallyOwnedUnlessCommented(t *testing.T) {
	// Arrange: trailing commas make it JSONC, but nothing in it is the consumer's.
	path := writeFixture(t, "mcp.jsonc", "{\n  \"mcpServers\": {},\n}\n")

	// Act
	result, err := jsonmerge.Apply(path, serverOwned())

	// Assert
	require.NoError(t, err)
	assert.False(t, result.PartiallyOwned)

	// A comment makes the file the consumer's.
	commented := writeFixture(t, "mcp2.jsonc", "{\n  // mine\n  \"mcpServers\": {},\n}\n")
	result, err = jsonmerge.Apply(commented, serverOwned())
	require.NoError(t, err)
	assert.True(t, result.PartiallyOwned)
}

func TestUnmerge_JSONC(t *testing.T) {
	tests := []struct {
		name      string
		original  string
		owned     []jsonmerge.OwnedKey
		wantEmpty bool
	}{
		{
			name:     "appended member round-trips to the original bytes",
			original: "{\n  // editor\n  \"a\": 1, // px\n  \"b\": 2\n}\n",
			owned:    []jsonmerge.OwnedKey{{Name: "mcpServers", Value: map[string]any{"gen": map[string]any{"command": "x"}}}},
		},
		{
			name:     "trailing comma style round-trips",
			original: "{\n  \"a\": 1,\n}\n",
			owned:    serverOwned(),
		},
		{
			name:     "members round-trip beside a hand-written server",
			original: "{\n  \"mcpServers\": {\n    // mine\n    \"mine\": {\"command\": \"m\"},\n  },\n}\n",
			owned:    serverOwned(),
		},
		{
			name:     "nested path round-trips",
			original: "{\n  // c\n  \"mcp\": {\n    \"timeout\": 5 // s\n  }\n}\n",
			owned:    []jsonmerge.OwnedKey{{Path: []string{"mcp", "servers"}, Value: map[string]any{"a": 1}}},
		},
		{
			name:     "inline document round-trips",
			original: `{"a": 1 /* c */}`,
			owned:    []jsonmerge.OwnedKey{{Name: "b", Value: 2}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := writeFixture(t, "settings.jsonc", tt.original)
			applied, err := jsonmerge.Apply(path, tt.owned)
			require.NoError(t, err)
			require.NotEqual(t, tt.original, applied.Body)

			// Act
			unmerged, err := jsonmerge.UnmergeDocument(path, applied.Body, applied.Claims)

			// Assert
			require.NoError(t, err)
			assert.True(t, unmerged.Changed)
			assert.False(t, unmerged.Empty)
			assert.Equal(t, tt.original, unmerged.Body)
		})
	}
}

func TestUnmerge_JSONCKeepsEditedValuesAndReportsEmpty(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.jsonc")
	owned := []jsonmerge.OwnedKey{{Name: "mcpServers", Value: map[string]any{"gen": map[string]any{"command": "x"}}, Members: true}}
	applied, err := jsonmerge.Apply(path, owned)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte(applied.Body), 0o644))

	// A freshly created document is strict JSON; editing it by hand to add a
	// comment turns it into JSONC that still contains only ai-rulez's entry.
	commented := "// generated\n" + applied.Body
	_, err = jsonmerge.UnmergeDocument(path, commented, applied.Claims)
	require.NoError(t, err)

	// Act: the entry was edited, so it is the user's now.
	edited := "// generated\n{\n  \"mcpServers\": {\n    \"gen\": {\"command\": \"mine\"}\n  }\n}\n"
	kept, err := jsonmerge.UnmergeDocument(path, edited, applied.Claims)
	require.NoError(t, err)
	assert.False(t, kept.Changed)
	assert.Equal(t, [][]string{{"mcpServers", "gen"}}, kept.Kept)

	// Act: untouched, the entry goes, the comment stays, so the file is not Empty.
	removed, err := jsonmerge.UnmergeDocument(path, commented, applied.Claims)

	// Assert
	require.NoError(t, err)
	assert.True(t, removed.Changed)
	assert.False(t, removed.Empty, "a comment is the consumer's content")
	assert.Equal(t, "// generated\n{}\n", removed.Body)
}

func TestUnmerge_JSONCElements(t *testing.T) {
	// Arrange
	doc := "{\n  \"instructions\": [\n    \"mine\", // keep\n    \"ours\",\n  ],\n}\n"
	claims := []jsonmerge.Claim{{Path: []string{"instructions"}, Elements: []any{"ours"}}}

	// Act
	result, err := jsonmerge.UnmergeDocument("opencode.jsonc", doc, claims)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "{\n  \"instructions\": [\n    \"mine\", // keep\n  ],\n}\n", result.Body)
}

func TestUnmerge_JSONCMalformed(t *testing.T) {
	// Act
	_, err := jsonmerge.UnmergeDocument("x.jsonc", "{ // c\n \"a\": }", []jsonmerge.Claim{{Path: []string{"a"}}})

	// Assert
	require.Error(t, err)
}
