package jsonmerge_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

// TestApplyUnmerge_RestoresEveryDocumentShapeByteForByte pins the promise that
// generate then clean returns a pre-existing document unchanged, whatever its layout.
func TestApplyUnmerge_RestoresEveryDocumentShapeByteForByte(t *testing.T) {
	docs := map[string]string{
		"one line no newline":      `{"model":"opus","env":{"A":"1"}}`,
		"one line newline":         "{\"model\":\"opus\",\"env\":{\"A\":\"1\"}}\n",
		"pretty":                   "{\n  \"model\": \"opus\",\n  \"env\": {\n    \"A\": \"1\"\n  }\n}\n",
		"pretty no final newline":  "{\n  \"model\": \"opus\"\n}",
		"four space indent":        "{\n    \"model\": \"opus\"\n}\n",
		"tabs":                     "{\n\t\"model\": \"opus\"\n}\n",
		"jsonc comment":            "{\n  // mine\n  \"model\": \"opus\"\n}\n",
		"jsonc one line comment":   "/* c */ {\"model\":\"opus\"}",
		"empty object":             "{}",
		"empty object newline":     "{}\n",
		"spaced one line":          `{ "theme": "dark" }`,
		"minified with spaces":     `{ "model": "opus", "env": { "A": "1" } }`,
		"crlf":                     "{\r\n  \"model\": \"opus\"\r\n}\r\n",
		"existing permissions key": `{"permissions":{"allow":["Bash"]},"model":"opus"}`,
	}
	owned := []jsonmerge.OwnedKey{
		{Path: []string{"permissions", "deny"}, Value: []any{"Read(.env)"}, Elements: []any{"Read(.env)"}},
		{Path: []string{"hooks"}, Value: map[string]any{"Stop": []any{map[string]any{"command": "x"}}}},
		{Path: []string{"mcpServers"}, Members: true, Value: map[string]any{"s": map[string]any{"command": "npx", "args": []any{"-y"}}}},
		{Path: []string{"context", "fileName"}, Value: []any{"GEMINI.md"}, Elements: []any{"GEMINI.md"}},
	}
	for name, doc := range docs {
		t.Run(name, func(t *testing.T) {
			path := writeFixture(t, "settings.json", doc)

			applied, err := jsonmerge.Apply(path, owned)
			require.NoError(t, err)
			restored, err := jsonmerge.UnmergeDocument(path, applied.Body, applied.Claims)

			require.NoError(t, err)
			require.True(t, restored.Changed)
			assert.Equal(t, doc, restored.Body)
		})
	}
}
