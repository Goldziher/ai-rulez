package jsonmerge_test

import (
	"encoding/json"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyThenUnmerge_RestoresStrictJSONByteForByte(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{"one-line nested object", "{\n  \"model\": \"x\",\n  \"permissions\": { \"allow\": [\"Read\", \"Edit\"] }\n}\n"},
		{"one-line array", "{\n    \"model\": \"x\",\n    \"permissions\": {\n        \"allow\": [\"Read\", \"Edit\"]\n    }\n}\n"},
		{"one-line nested object, CRLF", "{\r\n  \"permissions\": { \"allow\": [\"Read\", \"Edit\"] }\r\n}\r\n"},
	}
	owned := []jsonmerge.OwnedKey{
		{Path: []string{"permissions", "allow"}, Value: []any{"Read", "Edit", "Bash(ls)"}, Elements: []any{"Bash(ls)"}},
		{Path: []string{"permissions", "deny"}, Value: []any{"Bash(rm)"}, Elements: []any{"Bash(rm)"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			merged, err := jsonmerge.ApplyDocument("s.json", tt.doc, owned)
			require.NoError(t, err)
			require.NotEqual(t, tt.doc, merged.Body)

			// Act: the claims go through a manifest round trip, as clean reads them.
			encoded, err := json.Marshal(merged.Claims)
			require.NoError(t, err)
			var claims []jsonmerge.Claim
			require.NoError(t, json.Unmarshal(encoded, &claims))
			restored, err := jsonmerge.UnmergeDocument("s.json", merged.Body, claims)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.doc, restored.Body)
		})
	}
}
