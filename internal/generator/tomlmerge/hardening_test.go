package tomlmerge_test

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/generator/tomlmerge"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func genServer() []tomlmerge.OwnedKey {
	return servers(map[string]any{"gen": map[string]any{"command": "x"}})
}

func decode(t *testing.T, doc string) map[string]any {
	t.Helper()
	tree := map[string]any{}
	require.NoError(t, toml.Unmarshal([]byte(doc), &tree), doc)
	return tree
}

func roundTrip(t *testing.T, doc string, owned []tomlmerge.OwnedKey) (applied, restored string) {
	t.Helper()
	res, err := tomlmerge.ApplyDocument("c.toml", doc, owned)
	require.NoError(t, err)
	again, err := tomlmerge.ApplyDocument("c.toml", res.Body, owned)
	require.NoError(t, err)
	assert.Equal(t, res.Body, again.Body, "apply is idempotent")
	un, err := tomlmerge.UnmergeDocument("c.toml", res.Body, res.Claims)
	require.NoError(t, err)
	return res.Body, un.Body
}

func TestRoundTrip_Formats(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{"CRLF", "a = 1\r\n# c\r\n[b]\r\nx = 2\r\n"},
		{"CRLF with servers", "[mcp_servers.mine]\r\ncommand = \"m\"\r\n"},
		{"no trailing newline", "a = 1\nb = 2"},
		{"no trailing newline table", "a = 1\n[t]\nx = 1"},
		{"BOM", "\ufeffa = 1\n"},
		{"BOM CRLF", "\ufeffa = 1\r\nb = 2\r\n"},
		{"multi-line string at EOF", "a = 1\nnotes = \"\"\"\nline # one\n\"\"\"\n"},
		{"multi-line string at EOF no newline", "a = 1\nnotes = '''\nraw\n'''"},
		{"quoted table header", "[\"mcp_servers\".mine]\ncommand = \"m\"\n"},
		{"dotted keys", "mcp_servers.mine.command = \"m\"\nother = 1\n"},
		{"dotted key in table", "[mcp_servers]\nmine.command = \"m\"\n"},
		{"array of tables elsewhere", "[[products]]\nname = \"a\"\n\n[[products]]\nname = \"b\"\n"},
		{"only comments", "# nothing here\n"},
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

func TestApply_QuotedAndBareKeysAreTheSame(t *testing.T) {
	doc := "[\"mcp_servers\".\"gen\"]\ncommand = \"old\"\n\n[mcp_servers.mine]\ncommand = \"m\"\n"

	res, err := tomlmerge.ApplyDocument("c.toml", doc, genServer())

	require.NoError(t, err)
	tree := decode(t, res.Body)
	assert.Equal(t, map[string]any{"command": "x"}, tree["mcp_servers"].(map[string]any)["gen"])
	assert.Equal(t, map[string]any{"command": "m"}, tree["mcp_servers"].(map[string]any)["mine"])
	assert.Equal(t, 1, strings.Count(res.Body, "gen"))
}

func TestApply_OwnedKeyUnderUserInlineTableIsRefused(t *testing.T) {
	doc := "mcp_servers = { mine = { command = \"m\" } }\n"

	_, err := tomlmerge.ApplyDocument("c.toml", doc, genServer())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "inline table")
}

func TestUnmerge_OwnedKeyUnderUserInlineTableIsLeftAlone(t *testing.T) {
	doc := "mcp_servers = { gen = { command = \"x\" }, mine = { command = \"m\" } }\n"
	sum := tomlmerge.Claim{Path: []string{"mcp_servers", "gen"}}
	inlineList := "t = { list = [\"x\", \"y\"] }\n"

	un, err := tomlmerge.UnmergeDocument("c.toml", doc, []tomlmerge.Claim{sum})
	require.NoError(t, err)
	assert.False(t, un.Changed)

	un, err = tomlmerge.UnmergeDocument("c.toml", inlineList, []tomlmerge.Claim{{Path: []string{"t", "list"}, Elements: []any{"x"}}})
	require.NoError(t, err)
	assert.False(t, un.Changed)
}

func TestUnmerge_KeepsUserEmptyParentHeader(t *testing.T) {
	doc := "[mcp_servers]\n\n[other]\nx = 1\n"
	owned := []tomlmerge.OwnedKey{{Path: []string{"mcp_servers", "flag"}, Value: true}}

	applied, restored := roundTrip(t, doc, owned)

	assert.NotEqual(t, doc, applied)
	assert.Equal(t, doc, restored)
}

func TestUnmerge_DropsHeaderApplyCreated(t *testing.T) {
	doc := "other = 1\n"
	owned := []tomlmerge.OwnedKey{{Path: []string{"tools", "flag"}, Value: true}}

	_, restored := roundTrip(t, doc, owned)

	assert.Equal(t, doc, restored)
}

func TestApply_ElementsKeepInlineArrayOfTables(t *testing.T) {
	doc := "hooks = [{ name = \"mine\", cmd = \"a\" }]\n"
	elem := map[string]any{"name": "gen", "cmd": "b"}
	owned := []tomlmerge.OwnedKey{{
		Name:     "hooks",
		Value:    []any{map[string]any{"name": "mine", "cmd": "a"}, elem},
		Elements: []any{elem},
	}}

	res, err := tomlmerge.ApplyDocument("c.toml", doc, owned)
	require.NoError(t, err)
	assert.NotContains(t, res.Body, "[[hooks]]")
	assert.True(t, res.PartiallyOwned, "the user's element makes the document theirs")

	un, err := tomlmerge.UnmergeDocument("c.toml", res.Body, res.Claims)
	require.NoError(t, err)
	assert.NotContains(t, un.Body, "[[hooks]]")
	assert.Equal(t, decode(t, doc), decode(t, un.Body))
}

func TestApply_ElementsOnlyOwnedIsNotPartial(t *testing.T) {
	elem := map[string]any{"name": "gen"}
	owned := []tomlmerge.OwnedKey{{Name: "hooks", Value: []any{elem}, Elements: []any{elem}}}

	res, err := tomlmerge.ApplyDocument("c.toml", "# c\n", owned)
	require.NoError(t, err)
	assert.True(t, res.PartiallyOwned, "a comment")
	res, err = tomlmerge.ApplyDocument("c.toml", "hooks = [{ name = \"gen\" }]\n", owned)
	require.NoError(t, err)
	assert.False(t, res.PartiallyOwned)
}

func TestApply_OwnedValuesWithEscapes(t *testing.T) {
	value := map[string]any{
		"quote":   `say "hi" and 'bye'`,
		"newline": "a\nb\n",
		"unicode": "h\u00e9llo \U0001F600",
		"control": "bell\x07tab\tdel\x7fend",
		"hash":    "x # y",
		"backsl":  `C:\path\n`,
		"empty":   "",
		"q key":   "spaced",
		"a.b":     "dotted",
	}

	applied, restored := roundTrip(t, "a = 1\n", servers(map[string]any{"gen": value}))

	assert.Equal(t, value, decode(t, applied)["mcp_servers"].(map[string]any)["gen"])
	assert.Equal(t, "a = 1\n", restored)
}

func TestApply_IntegralFloatsAreRenderedAsIntegers(t *testing.T) {
	owned := []tomlmerge.OwnedKey{{Name: "limits", Value: map[string]any{"n": float64(5), "f": 1.5, "arr": []any{float64(2)}}}}

	res, err := tomlmerge.ApplyDocument("c.toml", "a = 1\n", owned)

	require.NoError(t, err)
	assert.Contains(t, res.Body, "n = 5\n")
	assert.NotContains(t, res.Body, "5.0")
	assert.Contains(t, res.Body, "f = 1.5")
	assert.Contains(t, res.Body, "arr = [2]")
}

func TestApply_RefusesArrayOfTablesAncestor(t *testing.T) {
	doc := "[[mcp_servers]]\nname = \"a\"\n"

	_, err := tomlmerge.ApplyDocument("c.toml", doc, genServer())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "array of tables")
}

func TestApply_DottedKeysAndTablesCoexist(t *testing.T) {
	doc := "mcp_servers.mine.command = \"m\"\n"

	res, err := tomlmerge.ApplyDocument("c.toml", doc, genServer())

	require.NoError(t, err)
	tree := decode(t, res.Body)["mcp_servers"].(map[string]any)
	assert.Equal(t, map[string]any{"command": "m"}, tree["mine"])
	assert.Equal(t, map[string]any{"command": "x"}, tree["gen"])
}
