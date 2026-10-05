package yamlmerge_test

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/generator/yamlmerge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func genServer() []yamlmerge.OwnedKey {
	return servers(map[string]any{"gen": map[string]any{"command": "x"}})
}

func decode(t *testing.T, doc string) map[string]any {
	t.Helper()
	tree := map[string]any{}
	require.NoError(t, yaml.Unmarshal([]byte(doc), &tree), doc)
	return tree
}

// roundTrip applies owned to doc, unmerges the recorded claims and returns the
// applied body and the unmerged one.
func roundTrip(t *testing.T, doc string, owned []yamlmerge.OwnedKey) (applied, restored string) {
	t.Helper()
	res, err := yamlmerge.ApplyDocument("c.yaml", doc, owned)
	require.NoError(t, err)
	again, err := yamlmerge.ApplyDocument("c.yaml", res.Body, owned)
	require.NoError(t, err)
	assert.Equal(t, res.Body, again.Body, "apply is idempotent")
	un, err := yamlmerge.UnmergeDocument("c.yaml", res.Body, res.Claims)
	require.NoError(t, err)
	return res.Body, un.Body
}

func TestMemberSpan_ColumnZeroCommentInsideOwnedBlock(t *testing.T) {
	doc := "mcp:\n  servers:\n    gen:\n      command: old\n# note at column zero\n      args: [a]\n" +
		"    mine:\n      command: m\nother: 1\n"

	// Act
	res, err := yamlmerge.ApplyDocument("c.yaml", doc, genServer())

	// Assert
	require.NoError(t, err)
	tree := decode(t, res.Body)
	assert.Equal(t, map[string]any{"command": "x"}, tree["mcp"].(map[string]any)["servers"].(map[string]any)["gen"])
	assert.Contains(t, res.Body, "mine:")
	assert.Contains(t, res.Body, "other: 1")

	// Unmerge removes the whole block, interior comment included, and nothing else.
	un, err := yamlmerge.UnmergeDocument("c.yaml", doc, []yamlmerge.Claim{
		{Path: []string{"mcp", "servers", "gen"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "mcp:\n  servers:\n    mine:\n      command: m\nother: 1\n", un.Body)
}

func TestMemberSpan_CommentBetweenMembersStaysOutOfSpan(t *testing.T) {
	doc := "mcp:\n  servers:\n    gen:\n      command: old\n# about mine\n    mine:\n      command: m\n"

	res, err := yamlmerge.ApplyDocument("c.yaml", doc, genServer())

	require.NoError(t, err)
	assert.Contains(t, res.Body, "# about mine\n    mine:")
	un, err := yamlmerge.UnmergeDocument("c.yaml", res.Body, res.Claims)
	require.NoError(t, err)
	assert.Equal(t, "mcp:\n  servers:\n# about mine\n    mine:\n      command: m\n", un.Body)
}

func TestMemberSpan_MultiLineFlowValues(t *testing.T) {
	doc := "servers: [\n  a,\n  b\n]\nmcp:\n  servers:\n    gen: {\n      command: old\n    }\n    mine: {command: m}\n"
	owned := append(genServer(), yamlmerge.OwnedKey{Name: "servers", Value: []string{"z"}})

	res, err := yamlmerge.ApplyDocument("c.yaml", doc, owned)

	require.NoError(t, err)
	tree := decode(t, res.Body)
	assert.Equal(t, []any{"z"}, tree["servers"])
	srv := tree["mcp"].(map[string]any)["servers"].(map[string]any)
	assert.Equal(t, map[string]any{"command": "x"}, srv["gen"])
	assert.Equal(t, map[string]any{"command": "m"}, srv["mine"])

	un, err := yamlmerge.UnmergeDocument("c.yaml", doc, []yamlmerge.Claim{{Path: []string{"servers"}}})
	require.NoError(t, err)
	assert.Equal(t, "mcp:\n  servers:\n    gen: {\n      command: old\n    }\n    mine: {command: m}\n", un.Body)
}

func TestUnmerge_KeepsUserParents(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{"user null parent", "mcp:\n  servers:\nother: 1\n"},
		{"user comment in parent", "mcp:\n  servers: # my servers\n    # keep me\nother: 1\n"},
		{"user empty flow parent", "mcp:\n  servers: {}\nother: 1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applied, restored := roundTrip(t, tt.doc, genServer())

			assert.NotEqual(t, tt.doc, applied)
			if strings.Contains(tt.doc, "{}") {
				assert.Equal(t, decode(t, "mcp:\n  servers:\nother: 1\n"), decode(t, restored))
				return
			}
			assert.Equal(t, tt.doc, restored)
		})
	}
}

func TestUnmerge_RemovesAncestorsApplyCreated(t *testing.T) {
	doc := "other: 1\n"
	_, restored := roundTrip(t, doc, genServer())
	assert.Equal(t, doc, restored)
}

func TestApply_PartiallyOwnedOnAnyComment(t *testing.T) {
	for _, doc := range []string{
		"mcp:\n  servers:\n    gen:\n      command: x\n# trailing\n",
		"# head\nmcp:\n  servers:\n    gen:\n      command: x\n",
	} {
		res, err := yamlmerge.ApplyDocument("c.yaml", doc, genServer())
		require.NoError(t, err, doc)
		assert.True(t, res.PartiallyOwned, doc)
	}
	res, err := yamlmerge.ApplyDocument("c.yaml", "mcp:\n  servers:\n    gen:\n      command: \"a#b\"\n", genServer())
	require.NoError(t, err)
	assert.False(t, res.PartiallyOwned, "a # inside a value is not a comment")
}

func TestApply_RefusesAmbiguousLineBreaks(t *testing.T) {
	for name, doc := range map[string]string{
		"lone CR": "a: 1\rb: 2\n",
		"NEL":     "a: 1\u0085b: 2\n",
		"LS":      "a: 1 b: 2\n",
		"PS":      "a: 1 b: 2\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := yamlmerge.ApplyDocument("c.yaml", doc, genServer())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "line break")
			_, err = yamlmerge.UnmergeDocument("c.yaml", doc, []yamlmerge.Claim{{Path: []string{"a"}}})
			require.Error(t, err)
		})
	}
}

func TestApply_RefusesAnchorsAndAliases(t *testing.T) {
	for name, doc := range map[string]string{
		"anchor": "base: &b {x: 1}\nother: 2\n",
		"alias":  "base: &b {x: 1}\nuse: *b\n",
		"merge":  "base: &b {x: 1}\nuse:\n  <<: *b\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := yamlmerge.ApplyDocument("c.yaml", doc, genServer())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "anchor")
		})
	}
}

func TestRoundTrip_Formats(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{"CRLF", "a: 1\r\n# c\r\nb:\r\n  - x\r\n"},
		{"CRLF nested", "mcp:\r\n  servers:\r\n    mine:\r\n      command: m\r\n"},
		{"no trailing newline", "a: 1\nb: 2"},
		{"no trailing newline comment", "a: 1\n# end"},
		{"BOM", "\ufeffa: 1\n"},
		{"BOM CRLF", "\ufeffa: 1\r\nb: 2\r\n"},
		{"document marker", "---\na: 1\n"},
		{"document marker comment", "# top\n---\na: 1 # c\n"},
		{"block scalars", "notes: |\n  one\n\n  # not a comment\nfolded: >\n  a\n  b\nkeep: |+\n  k\n\nlast: 1\n"},
		{"block scalar keep at EOF", "a: 1\nkeep: |+\n  k\n\n"},
		{"block scalar strip at EOF", "a: 1\nstrip: >-\n  s\n  t\n"},
		{"quoted keys", "\"a b\": 1\n'c': 2\n"},
		{"empty with comment", "# only a comment\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applied, restored := roundTrip(t, tt.doc, genServer())
			assert.NotEqual(t, tt.doc, applied)
			assert.Equal(t, tt.doc, restored)
			assert.Equal(t, strings.HasPrefix(tt.doc, "\ufeff"), strings.HasPrefix(applied, "\ufeff"))
		})
	}
}

func TestApply_OwnedValuesWithEscapes(t *testing.T) {
	value := map[string]any{
		"quote":   `say "hi" and 'bye'`,
		"newline": "a\nb\n",
		"unicode": "héllo é \U0001F600",
		"control": "bell\x07tab\tend",
		"hash":    "x # y",
		"colon":   "a: b",
		"lead":    "- dash",
		"nullish": "null",
		"num":     "123",
		"empty":   "",
		"bool":    "true",
	}
	owned := servers(map[string]any{"gen": value})

	applied, restored := roundTrip(t, "a: 1\n", owned)

	tree := decode(t, applied)
	assert.Equal(t, value, tree["mcp"].(map[string]any)["servers"].(map[string]any)["gen"])
	assert.Equal(t, "a: 1\n", restored)
}

func TestApply_RefusesDamagingEdit(t *testing.T) {
	// A user's keep-chomped scalar must survive an append at the end of the file.
	doc := "a: 1\nkeep: |+\n  k\n\n"
	res, err := yamlmerge.ApplyDocument("c.yaml", doc, genServer())
	require.NoError(t, err)
	assert.Equal(t, "k\n\n", decode(t, res.Body)["keep"])
}

func TestApply_ElementsKeepFlowStyleAndMarkSharedArrays(t *testing.T) {
	doc := "hooks: [mine]\nother: 1\n"
	owned := []yamlmerge.OwnedKey{{Name: "hooks", Value: []any{"mine", "gen"}, Elements: []any{"gen"}}}

	res, err := yamlmerge.ApplyDocument("c.yaml", doc, owned)

	require.NoError(t, err)
	assert.Equal(t, "hooks: [mine, gen]\nother: 1\n", res.Body)
	assert.True(t, res.PartiallyOwned, "the user's element makes the document theirs")
	un, err := yamlmerge.UnmergeDocument("c.yaml", res.Body, res.Claims)
	require.NoError(t, err)
	assert.Equal(t, "hooks: [mine]\nother: 1\n", un.Body)
}

func TestApply_ElementsOnlyOwnedIsNotPartial(t *testing.T) {
	owned := []yamlmerge.OwnedKey{{Name: "hooks", Value: []any{"gen"}, Elements: []any{"gen"}}}

	res, err := yamlmerge.ApplyDocument("c.yaml", "hooks:\n  - gen\n", owned)

	require.NoError(t, err)
	assert.False(t, res.PartiallyOwned)
}
