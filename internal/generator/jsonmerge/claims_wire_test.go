package jsonmerge_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

func TestClaimLegacyEqualsBecomesDigest(t *testing.T) {
	// Arrange
	legacy := `{"path":["mcp","srv"],"equals":{"env":{"TOKEN":"hunter2-secret"}}}`
	var claim jsonmerge.Claim

	// Act
	require.NoError(t, json.Unmarshal([]byte(legacy), &claim))
	written, err := json.Marshal(claim)
	require.NoError(t, err)

	// Assert
	assert.Nil(t, claim.Equals)
	assert.NotEmpty(t, claim.Sum)
	assert.NotContains(t, string(written), "hunter2-secret")
	assert.True(t, claim.Matches(json.RawMessage(`{"env":{"TOKEN":"hunter2-secret"}}`)), "comparison semantics are kept")
	assert.False(t, claim.Matches(json.RawMessage(`{"env":{"TOKEN":"edited"}}`)))
}

func TestClaimInMemoryEqualsIsNotPersisted(t *testing.T) {
	claim := jsonmerge.Claim{Path: []string{"mcp", "srv"}, Equals: map[string]any{"env": map[string]any{"K": "s3cret-value"}}}

	written, err := json.Marshal(claim)

	require.NoError(t, err)
	assert.NotContains(t, string(written), "s3cret-value")
	assert.Contains(t, string(written), `"sum"`)
}

func TestClaimHeaderTextSurvivesTheManifest(t *testing.T) {
	claim := jsonmerge.Claim{Path: []string{"header"}, Equals: "---\nname: x\n---"}

	written, err := json.Marshal(claim)
	require.NoError(t, err)
	var back jsonmerge.Claim
	require.NoError(t, json.Unmarshal(written, &back))

	assert.Equal(t, "---\nname: x\n---", back.Equals)
}

func TestUnmergeRemovesOnlyAsManyIdenticalElementsAsWereAdded(t *testing.T) {
	tests := []struct {
		name     string
		document string
		claim    jsonmerge.Claim
		want     []any
	}{
		{
			name:     "a hand-written identical element stays",
			document: `{"allow": ["Bash(ls)", "Bash(ls)", "mine"]}`,
			claim:    jsonmerge.Claim{Path: []string{"allow"}, Elements: []any{"Bash(ls)"}},
			want:     []any{"Bash(ls)", "mine"},
		},
		{
			name:     "two claimed copies remove two",
			document: `{"allow": ["Bash(ls)", "Bash(ls)", "mine"]}`,
			claim:    jsonmerge.Claim{Path: []string{"allow"}, Elements: []any{"Bash(ls)", "Bash(ls)"}},
			want:     []any{"mine"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := filepath.Join(t.TempDir(), "settings.json")
			require.NoError(t, os.WriteFile(path, []byte(tt.document), 0o600))
			wire, err := json.Marshal(tt.claim)
			require.NoError(t, err)
			var claim jsonmerge.Claim
			require.NoError(t, json.Unmarshal(wire, &claim))

			// Act
			result, err := jsonmerge.Unmerge(path, []jsonmerge.Claim{claim})

			// Assert
			require.NoError(t, err)
			var got map[string][]any
			require.NoError(t, json.Unmarshal([]byte(result.Body), &got))
			assert.Equal(t, tt.want, got["allow"])
		})
	}
}
