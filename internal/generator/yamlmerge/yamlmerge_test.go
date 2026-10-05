package yamlmerge_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/yamlmerge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func writeFixture(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func servers(entries map[string]any) []yamlmerge.OwnedKey {
	return []yamlmerge.OwnedKey{{Path: []string{"mcp", "servers"}, Value: entries, Members: true}}
}

const handAuthored = `# my config
model: gpt-5 # default

editor:
    tab: 4 # spaces
    notes: |
        multi line
        # not a comment

list:
- a
- b # second
`

func TestApply(t *testing.T) {
	tests := []struct {
		name  string
		doc   string
		owned []yamlmerge.OwnedKey
		want  string
	}{
		{
			name:  "adds a nested owned key at the end and keeps everything else",
			doc:   handAuthored,
			owned: servers(map[string]any{"gen": map[string]any{"command": "x", "args": []string{"-y"}}}),
			want: handAuthored + `mcp:
    servers:
        gen:
            args:
                - -y
            command: x
`,
		},
		{
			name: "replaces an owned member in place and keeps its trailing comment",
			doc: `a: 1
# the model
model: old # pinned

b: 2
`,
			owned: []yamlmerge.OwnedKey{{Name: "model", Value: "new"}},
			want: `a: 1
# the model
model: new # pinned

b: 2
`,
		},
		{
			name: "replaces a mapping member including nested lines, stops at the next key",
			doc: `mcp:
  servers:
    gen:
      command: stale
      args:
        - old

    mine:
      command: mine # keep

  timeout: 5
other: 1
`,
			owned: servers(map[string]any{"gen": map[string]any{"command": "x"}}),
			want: `mcp:
  servers:
    gen:
      command: x

    mine:
      command: mine # keep

  timeout: 5
other: 1
`,
		},
		{
			name: "new member goes after the last sibling inside its parent",
			doc: `mcp:
  servers:
    mine:
      command: mine
  # trailing comment of mcp
other: 1
`,
			owned: servers(map[string]any{"gen": map[string]any{"command": "x"}}),
			want: `mcp:
  servers:
    mine:
      command: mine
    gen:
      command: x
  # trailing comment of mcp
other: 1
`,
		},
		{
			name:  "null parent is filled",
			doc:   "mcp:\nother: 1\n",
			owned: servers(map[string]any{"gen": map[string]any{"command": "x"}}),
			want:  "mcp:\n  servers:\n    gen:\n      command: x\nother: 1\n",
		},
		{
			name:  "empty flow mapping becomes block",
			doc:   "mcp:\n  servers: {}\nother: 1\n",
			owned: servers(map[string]any{"gen": map[string]any{"command": "x"}}),
			want:  "mcp:\n  servers:\n    gen:\n      command: x\nother: 1\n",
		},
		{
			name:  "flow mapping with entries stays flow",
			doc:   "mcp:\n  servers: {mine: {command: m}}\nother: 1\n",
			owned: servers(map[string]any{"gen": map[string]any{"command": "x"}}),
			want:  "mcp:\n  servers: {mine: {command: m}, gen: {command: x}}\nother: 1\n",
		},
		{
			name:  "sequence at key indentation belongs to its key",
			doc:   "list:\n- a\n- b\nz: 1\n",
			owned: []yamlmerge.OwnedKey{{Name: "list", Value: []string{"c"}}},
			want:  "list:\n  - c\nz: 1\n",
		},
		{
			name:  "root with only comments",
			doc:   "# nothing here yet\n",
			owned: []yamlmerge.OwnedKey{{Name: "a", Value: 1}},
			want:  "# nothing here yet\na: 1\n",
		},
		{
			name:  "remove drops the member and the mapping it empties",
			doc:   "keep: 1\nmcp:\n  servers:\n    gen:\n      command: x\nz: 2\n",
			owned: []yamlmerge.OwnedKey{{Path: []string{"mcp", "servers", "gen"}, Remove: true}},
			want:  "keep: 1\nz: 2\n",
		},
		{
			name:  "remove of an absent key is a no-op",
			doc:   handAuthored,
			owned: []yamlmerge.OwnedKey{{Path: []string{"nope", "x"}, Remove: true}},
			want:  handAuthored,
		},
		{
			name:  "CRLF documents keep CRLF",
			doc:   "a: 1\r\nb: 2\r\n",
			owned: []yamlmerge.OwnedKey{{Name: "c", Value: map[string]any{"d": 1}}},
			want:  "a: 1\r\nb: 2\r\nc:\r\n  d: 1\r\n",
		},
		{
			name:  "document without a trailing newline",
			doc:   "a: 1",
			owned: []yamlmerge.OwnedKey{{Name: "b", Value: 2}},
			want:  "a: 1\nb: 2\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := writeFixture(t, "config.yaml", tt.doc)

			// Act
			result, err := yamlmerge.Apply(path, tt.owned)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.Body)
			var parsed map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(result.Body), &parsed), "result must be valid YAML")

			// Idempotent: a second pass over its own output changes nothing.
			again, err := yamlmerge.Apply(writeFixture(t, "again.yaml", result.Body), tt.owned)
			require.NoError(t, err)
			assert.Equal(t, result.Body, again.Body)
		})
	}
}

func TestApply_NewAndEmptyFiles(t *testing.T) {
	tests := []struct {
		name string
		doc  *string
	}{
		{"missing file", nil},
		{"whitespace only file", ptr("\n  \n")},
	}
	owned := []yamlmerge.OwnedKey{
		{Name: "model", Value: "opus"},
		{Path: []string{"mcp", "servers"}, Value: map[string]any{"b": map[string]any{"command": "b"}, "a": map[string]any{"command": "a"}}, Members: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := filepath.Join(t.TempDir(), "config.yaml")
			if tt.doc != nil {
				require.NoError(t, os.WriteFile(path, []byte(*tt.doc), 0o600))
			}

			// Act
			result, err := yamlmerge.Apply(path, owned)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, "mcp:\n  servers:\n    a:\n      command: a\n    b:\n      command: b\nmodel: opus\n", result.Body)
			assert.False(t, result.PartiallyOwned)
			assert.NotEmpty(t, result.Claims)
		})
	}
}

func ptr(s string) *string { return &s }

func TestApply_PartiallyOwned(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want bool
	}{
		{"only owned keys", "mcp:\n  servers:\n    gen:\n      command: old\n", false},
		{"a user key", "model: x\nmcp:\n  servers:\n    gen: {}\n", true},
		{"a user server beside ours", "mcp:\n  servers:\n    mine: {}\n", true},
		{"a sibling under an owned ancestor", "mcp:\n  timeout: 5\n", true},
		{"a comment", "# mine\nmcp:\n  servers:\n    gen: {}\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := writeFixture(t, "config.yaml", tt.doc)

			// Act
			result, err := yamlmerge.Apply(path, servers(map[string]any{"gen": map[string]any{"command": "x"}}))

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.PartiallyOwned)
		})
	}
}

func TestApply_Refusals(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{"malformed", "a: [1, 2\nb: : :\n"},
		{"tab indentation", "a:\n\tb: 1\n"},
		{"sequence root", "- a\n- b\n"},
		{"scalar root", "just text\n"},
		{"multiple documents", "a: 1\n---\nb: 2\n"},
		{"flow root", "{a: 1}\n"},
		{"owned parent is a scalar", "mcp: text\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := writeFixture(t, "config.yaml", tt.doc)

			// Act
			_, err := yamlmerge.Apply(path, servers(map[string]any{"gen": map[string]any{"command": "x"}}))

			// Assert
			require.Error(t, err)
			onDisk, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, tt.doc, string(onDisk))
		})
	}
}

func TestUnmerge_RoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		original string
		owned    []yamlmerge.OwnedKey
	}{
		{"appended nested member", handAuthored, servers(map[string]any{"gen": map[string]any{"command": "x"}})},
		{
			"member beside a user server",
			"mcp:\n  servers:\n    mine:\n      command: m\n\nother: 1\n",
			servers(map[string]any{"gen": map[string]any{"command": "x", "env": map[string]any{"K": "v"}}}),
		},
		{"root scalar", "# top\nmodel: a\n\n# footer\n", []yamlmerge.OwnedKey{{Name: "extra", Value: "x"}}},
		{"flow mapping holder", "mcp:\n  servers: {mine: {command: m}}\n", servers(map[string]any{"gen": map[string]any{"command": "x"}})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := writeFixture(t, "config.yaml", tt.original)
			applied, err := yamlmerge.Apply(path, tt.owned)
			require.NoError(t, err)
			require.NotEqual(t, tt.original, applied.Body)

			// Act
			unmerged, err := yamlmerge.UnmergeDocument(path, applied.Body, applied.Claims)

			// Assert
			require.NoError(t, err)
			assert.True(t, unmerged.Changed)
			assert.False(t, unmerged.Empty)
			assert.Equal(t, tt.original, unmerged.Body)
		})
	}
}

func TestUnmerge_GeneratedFileIsEmpty(t *testing.T) {
	// Arrange
	owned := servers(map[string]any{"gen": map[string]any{"command": "x"}})
	applied, err := yamlmerge.Apply(filepath.Join(t.TempDir(), "config.yaml"), owned)
	require.NoError(t, err)

	// Act
	unmerged, err := yamlmerge.UnmergeDocument("config.yaml", applied.Body, applied.Claims)

	// Assert
	require.NoError(t, err)
	assert.True(t, unmerged.Changed)
	assert.True(t, unmerged.Empty)
	assert.Empty(t, unmerged.Body)
}

func TestUnmerge_KeepsEditedValuesAndComments(t *testing.T) {
	// Arrange
	owned := servers(map[string]any{"gen": map[string]any{"command": "x"}})
	applied, err := yamlmerge.Apply("", owned)
	require.NoError(t, err)

	// Act: the user edited the entry, so it stays.
	kept, err := yamlmerge.UnmergeDocument("config.yaml",
		"mcp:\n  servers:\n    gen:\n      command: mine now\n", applied.Claims)
	require.NoError(t, err)

	// Act: a comment keeps the file from being reported empty.
	commented, err := yamlmerge.UnmergeDocument("config.yaml", "# mine\n"+applied.Body, applied.Claims)

	// Assert
	assert.False(t, kept.Changed)
	assert.Equal(t, [][]string{{"mcp", "servers", "gen"}}, kept.Kept)
	require.NoError(t, err)
	assert.True(t, commented.Changed)
	assert.False(t, commented.Empty)
	assert.Equal(t, "# mine\n", commented.Body)
}

func TestUnmerge_AloneAndElements(t *testing.T) {
	tests := []struct {
		name   string
		doc    string
		claims []yamlmerge.Claim
		want   string
		change bool
	}{
		{
			name:   "alone claim waits for a sole key",
			doc:    "x: 1\nnote: n\n",
			claims: []yamlmerge.Claim{{Path: []string{"note"}, Alone: true}},
		},
		{
			name:   "alone claim applies to a sole key",
			doc:    "note: n\n",
			claims: []yamlmerge.Claim{{Path: []string{"note"}, Alone: true}},
			change: true,
		},
		{
			name:   "elements are taken out of the user's list",
			doc:    "instructions:\n  - mine\n  - ours\nz: 1\n",
			claims: []yamlmerge.Claim{{Path: []string{"instructions"}, Elements: []any{"ours"}}},
			want:   "instructions:\n  - mine\nz: 1\n",
			change: true,
		},
		{
			name:   "a list left empty is removed",
			doc:    "x: 1\ninstructions:\n  - ours\n",
			claims: []yamlmerge.Claim{{Path: []string{"instructions"}, Elements: []any{"ours"}}},
			want:   "x: 1\n",
			change: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			result, err := yamlmerge.UnmergeDocument("config.yaml", tt.doc, tt.claims)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.change, result.Changed)
			if tt.change && tt.want != "" {
				assert.Equal(t, tt.want, result.Body)
			}
		})
	}
}

func TestUnmerge_MissingFileAndMalformed(t *testing.T) {
	// Act
	missing, err := yamlmerge.Unmerge(filepath.Join(t.TempDir(), "none.yaml"), []yamlmerge.Claim{{Path: []string{"a"}}})

	// Assert
	require.NoError(t, err)
	assert.False(t, missing.Changed)

	_, err = yamlmerge.UnmergeDocument("x.yaml", "a: [1\n", []yamlmerge.Claim{{Path: []string{"a"}}})
	require.Error(t, err)
}

func TestApply_ListReplacementKeepsUnchangedElementsVerbatim(t *testing.T) {
	// Arrange: the first element carries comments and keys in a deliberate order.
	existing := "# top\n" +
		"items:\n" +
		"  # first\n" +
		"  - name: a   # note\n" +
		"    zeta: 1\n" +
		"    alpha: 2\n" +
		"\n" +
		"  - name: b\n" +
		"other: 1\n"
	kept := map[string]any{"name": "a", "zeta": 1, "alpha": 2}
	added := map[string]any{"name": "c"}

	// Act: b is dropped, c is added, a is untouched.
	res, err := yamlmerge.ApplyDocument("c.yaml", existing, []yamlmerge.OwnedKey{{Name: "items", Value: []any{kept, added}, Elements: []any{added}}})

	// Assert
	require.NoError(t, err)
	want := "# top\n" +
		"items:\n" +
		"  # first\n" +
		"  - name: a   # note\n" +
		"    zeta: 1\n" +
		"    alpha: 2\n" +
		"\n" +
		"  - name: c\n" +
		"other: 1\n"
	assert.Equal(t, want, res.Body)
	assert.True(t, res.PartiallyOwned)
}
