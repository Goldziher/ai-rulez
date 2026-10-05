package jsonmerge_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tailscale/hujson"
)

func jsoncRoundTrip(t *testing.T, doc string, owned []jsonmerge.OwnedKey) (applied, restored string) {
	t.Helper()
	path := writeFixture(t, "settings.json", doc)
	res, err := jsonmerge.Apply(path, owned)
	require.NoError(t, err)
	writeBack := writeFixture(t, "again.json", res.Body)
	again, err := jsonmerge.Apply(writeBack, owned)
	require.NoError(t, err)
	assert.Equal(t, res.Body, again.Body, "apply is idempotent")
	un, err := jsonmerge.UnmergeDocument(path, res.Body, res.Claims)
	require.NoError(t, err)
	return res.Body, un.Body
}

func TestJSONC_RoundTripFormats(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{"CRLF", "{\r\n  // mine\r\n  \"a\": 1,\r\n  \"b\": [1, 2]\r\n}\r\n"},
		{"tab indented", "{\n\t// mine\n\t\"a\": 1,\n\t\"nested\": {\n\t\t\"x\": 1\n\t}\n}\n"},
		{"no trailing newline", "{\n  // mine\n  \"a\": 1\n}"},
		{"no trailing newline trailing comma", "{\n  \"a\": 1, // c\n}"},
		{"BOM", "\ufeff{\n  // mine\n  \"a\": 1\n}\n"},
		{"BOM CRLF", "\ufeff{\r\n  // mine\r\n  \"a\": 1\r\n}\r\n"},
		{"single line", "{ /* c */ \"a\": 1 }"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applied, restored := jsoncRoundTrip(t, tt.doc, serverOwned())
			assert.NotEqual(t, tt.doc, applied)
			assert.Equal(t, tt.doc, restored)
			assert.Equal(t, strings.HasPrefix(tt.doc, "\ufeff"), strings.HasPrefix(applied, "\ufeff"))
		})
	}
}

func TestStrictJSON_BOMIsKept(t *testing.T) {
	doc := "\ufeff{\n  \"a\": 1\n}\n"
	path := writeFixture(t, "settings.json", doc)

	res, err := jsonmerge.Apply(path, serverOwned())
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(res.Body, "\ufeff"))

	un, err := jsonmerge.UnmergeDocument(path, res.Body, res.Claims)
	require.NoError(t, err)
	assert.Equal(t, doc, un.Body)
}

func TestJSONC_ElementsWithUserElementsArePartial(t *testing.T) {
	elem := map[string]any{"name": "gen"}
	owned := []jsonmerge.OwnedKey{{Name: "hooks", Value: []any{map[string]any{"name": "mine"}, elem}, Elements: []any{elem}}}

	res, err := jsonmerge.Apply(writeFixture(t, "s.json", "{ // c\n  \"hooks\": [{\"name\": \"mine\"}]\n}\n"), owned)
	require.NoError(t, err)
	assert.True(t, res.PartiallyOwned)

	onlyOwned := []jsonmerge.OwnedKey{{Name: "hooks", Value: []any{elem}, Elements: []any{elem}}}
	res, err = jsonmerge.Apply(writeFixture(t, "s2.json", "{\n  \"hooks\": [{\"name\": \"gen\"}], // c\n}\n"), onlyOwned)
	require.NoError(t, err)
	assert.True(t, res.PartiallyOwned, "comment")

	merged := []jsonmerge.OwnedKey{{
		Name: "hooks", Value: []any{map[string]any{"name": "mine"}, elem}, Elements: []any{elem},
	}}
	res, err = jsonmerge.Apply(writeFixture(t, "s3.json", "{\"hooks\": [{\"name\": \"mine\"}]}\n"), merged)
	require.NoError(t, err)
	assert.True(t, res.PartiallyOwned, "user element in strict JSON")
}

func TestCheckPreserved(t *testing.T) {
	owned := []jsonmerge.OwnedKey{{Path: []string{"a", "b"}, Value: 1}}
	before := map[string]any{"keep": []any{1.0}, "a": map[string]any{"c": 2.0}}

	require.NoError(t, jsonmerge.CheckPreservedApply(before,
		map[string]any{"keep": []any{1.0}, "a": map[string]any{"c": 2.0, "b": 1.0}}, owned))
	require.Error(t, jsonmerge.CheckPreservedApply(before,
		map[string]any{"keep": []any{2.0}, "a": map[string]any{"c": 2.0, "b": 1.0}}, owned))
	require.Error(t, jsonmerge.CheckPreservedApply(before,
		map[string]any{"a": map[string]any{"b": 1.0}}, owned))
	require.Error(t, jsonmerge.CheckPreservedUnmerge(
		map[string]any{"a": map[string]any{"b": 1.0}, "x": 1.0},
		map[string]any{"x": 2.0}, []jsonmerge.Claim{{Path: []string{"a", "b"}}}))
	// Elements: only the claimed ones may go.
	require.NoError(t, jsonmerge.CheckPreservedUnmerge(
		map[string]any{"h": []any{"u", "o"}}, map[string]any{"h": []any{"u"}},
		[]jsonmerge.Claim{{Path: []string{"h"}, Elements: []any{"o"}}}))
	require.Error(t, jsonmerge.CheckPreservedUnmerge(
		map[string]any{"h": []any{"u", "o"}}, map[string]any{},
		[]jsonmerge.Claim{{Path: []string{"h"}, Elements: []any{"o"}}}))
}

func TestJSONC_OwnedValuesWithEscapes(t *testing.T) {
	value := map[string]any{
		"quote":   `say "hi"`,
		"newline": "a\nb",
		"unicode": "h\u00e9llo \U0001F600",
		"control": "bell\x07tab\tend",
		"slash":   `C:\path // not a comment`,
		"html":    "<a&b>",
	}
	owned := []jsonmerge.OwnedKey{{Name: "mcpServers", Value: map[string]any{"gen": value}, Members: true}}

	applied, restored := jsoncRoundTrip(t, "{\n  // mine\n  \"a\": 1\n}\n", owned)

	var tree map[string]any
	std, err := hujson.Standardize([]byte(applied))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(std, &tree))
	assert.Equal(t, value, tree["mcpServers"].(map[string]any)["gen"])
	assert.Equal(t, "{\n  // mine\n  \"a\": 1\n}\n", restored)
}
