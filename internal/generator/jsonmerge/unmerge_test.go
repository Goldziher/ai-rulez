package jsonmerge_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyClaims(t *testing.T) {
	tests := []struct {
		name  string
		owned []jsonmerge.OwnedKey
		want  []jsonmerge.Claim
	}{
		{
			name:  "scalar key is claimed whole",
			owned: []jsonmerge.OwnedKey{{Name: "effort", Value: "high"}},
			want:  []jsonmerge.Claim{{Path: []string{"effort"}, Sum: jsonmerge.Digest("high")}},
		},
		{
			name: "members claim each server by name, sorted",
			owned: []jsonmerge.OwnedKey{{
				Path: []string{"mcp", "servers"}, Members: true,
				Value: map[string]any{"b": 1, "a": 2},
			}},
			want: []jsonmerge.Claim{
				{Path: []string{"mcp", "servers", "a"}, Sum: jsonmerge.Digest(2)},
				{Path: []string{"mcp", "servers", "b"}, Sum: jsonmerge.Digest(1)},
			},
		},
		{
			name:  "empty members map claims the key",
			owned: []jsonmerge.OwnedKey{{Name: "mcpServers", Members: true, Value: map[string]any{}}},
			want:  []jsonmerge.Claim{{Path: []string{"mcpServers"}, Sum: jsonmerge.Digest(map[string]any{})}},
		},
		{
			name: "elements claim only what was added",
			owned: []jsonmerge.OwnedKey{{
				Path: []string{"instructions"}, Value: []any{"mine.md", "AGENTS.local.md"},
				Elements: []any{"AGENTS.local.md"},
			}},
			want: []jsonmerge.Claim{{Path: []string{"instructions"}, Elements: []any{"AGENTS.local.md"}}},
		},
		{
			name: "no elements added claims nothing",
			owned: []jsonmerge.OwnedKey{{
				Path: []string{"instructions"}, Value: []any{"mine.md"}, Elements: []any{},
			}},
			want: nil,
		},
		{
			name: "a created empty array is claimed",
			owned: []jsonmerge.OwnedKey{{
				Path: []string{"permissions", "allow"}, Value: []any{}, Elements: []any{}, Created: true,
			}},
			want: []jsonmerge.Claim{{Path: []string{"permissions", "allow"}, Elements: []any{}}},
		},
		{
			name:  "removal claims nothing",
			owned: []jsonmerge.OwnedKey{{Name: "x", Remove: true}},
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			result, err := jsonmerge.Apply("", tt.owned)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.Claims)
		})
	}
}

func TestUnmerge(t *testing.T) {
	tests := []struct {
		name        string
		doc         string
		claims      []jsonmerge.Claim
		want        string
		wantChanged bool
		wantEmpty   bool
		wantKept    [][]string
	}{
		{
			name: "removes a claimed server and keeps user keys and formatting",
			doc: `{
    "permissions": {"allow": ["Bash"]},
    "mcpServers": {
        "h": {"headers": {"Authorization": "Bearer s3cret"}}
    }
}
`,
			claims:      []jsonmerge.Claim{{Path: []string{"mcpServers", "h"}}},
			want:        "{\n    \"permissions\": {\"allow\": [\"Bash\"]}\n}\n",
			wantChanged: true,
		},
		{
			name:        "user server beside ours survives",
			doc:         `{"mcpServers": {"h": {}, "mine": {"command": "x"}}}`,
			claims:      []jsonmerge.Claim{{Path: []string{"mcpServers", "h"}}},
			want:        `{"mcpServers": {"mine": {"command": "x"}}}`,
			wantChanged: true,
		},
		{
			name:        "document with nothing else left is empty",
			doc:         `{"mcpServers": {"h": {}}}`,
			claims:      []jsonmerge.Claim{{Path: []string{"mcpServers", "h"}}},
			want:        "{}",
			wantChanged: true,
			wantEmpty:   true,
		},
		{
			name:        "elements are removed and the user's stay",
			doc:         `{"instructions": ["mine.md", "AGENTS.local.md"]}`,
			claims:      []jsonmerge.Claim{{Path: []string{"instructions"}, Elements: []any{"AGENTS.local.md", "./AGENTS.local.md"}}},
			want:        `{"instructions": ["mine.md"]}`,
			wantChanged: true,
		},
		{
			name:        "an emptied array drops the key and its empty parents",
			doc:         `{"context": {"fileName": ["GEMINI.local.md"]}, "theme": "dark"}`,
			claims:      []jsonmerge.Claim{{Path: []string{"context", "fileName"}, Elements: []any{"GEMINI.local.md"}}},
			want:        `{"theme": "dark"}`,
			wantChanged: true,
		},
		{
			name:        "equals guard keeps a value that is not ours",
			doc:         `{"$schema": "https://example.com/mine.json"}`,
			claims:      []jsonmerge.Claim{{Path: []string{"$schema"}, Equals: "https://opencode.ai/config.json"}},
			want:        "",
			wantChanged: false,
			wantKept:    [][]string{{"$schema"}},
		},
		{
			name:        "sum guard removes the value that was written",
			doc:         `{"mcpServers": {"h": {"command": "x"}, "mine": {}}}`,
			claims:      []jsonmerge.Claim{{Path: []string{"mcpServers", "h"}, Sum: jsonmerge.Digest(map[string]any{"command": "x"})}},
			want:        `{"mcpServers": {"mine": {}}}`,
			wantChanged: true,
		},
		{
			name:        "sum guard keeps a value the user edited",
			doc:         `{"mcpServers": {"h": {"command": "edited"}}}`,
			claims:      []jsonmerge.Claim{{Path: []string{"mcpServers", "h"}, Sum: jsonmerge.Digest(map[string]any{"command": "x"})}},
			wantChanged: false,
			wantKept:    [][]string{{"mcpServers", "h"}},
		},
		{
			name: "a later claim that matches clears the mismatch of an earlier one",
			doc:  `{"mcpServers": {"h": {"command": "new"}}}`,
			claims: []jsonmerge.Claim{
				{Path: []string{"mcpServers", "h"}, Sum: jsonmerge.Digest(map[string]any{"command": "old"})},
				{Path: []string{"mcpServers", "h"}, Sum: jsonmerge.Digest(map[string]any{"command": "new"})},
			},
			want:        "{}",
			wantChanged: true,
			wantEmpty:   true,
		},
		{
			name:        "alone claim is removed only when nothing else remains",
			doc:         `{"$schema": "https://opencode.ai/config.json", "model": "x"}`,
			claims:      []jsonmerge.Claim{{Path: []string{"$schema"}, Equals: "https://opencode.ai/config.json", Alone: true}},
			want:        "",
			wantChanged: false,
		},
		{
			name:        "alone claim goes when it is the last key",
			doc:         `{"$schema": "https://opencode.ai/config.json"}`,
			claims:      []jsonmerge.Claim{{Path: []string{"$schema"}, Equals: "https://opencode.ai/config.json", Alone: true}},
			want:        "{}",
			wantChanged: true,
			wantEmpty:   true,
		},
		{
			name:        "absent claim changes nothing",
			doc:         `{"a": 1}`,
			claims:      []jsonmerge.Claim{{Path: []string{"mcpServers", "h"}}},
			wantChanged: false,
		},
		{
			name:        "non-object ancestor is left alone",
			doc:         `{"mcpServers": []}`,
			claims:      []jsonmerge.Claim{{Path: []string{"mcpServers", "h"}}},
			wantChanged: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := filepath.Join(t.TempDir(), "settings.json")
			require.NoError(t, os.WriteFile(path, []byte(tt.doc), 0o644))

			// Act
			got, err := jsonmerge.Unmerge(path, tt.claims)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantChanged, got.Changed)
			assert.Equal(t, tt.wantEmpty, got.Empty)
			assert.Equal(t, tt.wantKept, got.Kept)
			if tt.wantChanged {
				assert.Equal(t, tt.want, got.Body)
			}
		})
	}
}

func TestUnmergeMissingFile(t *testing.T) {
	got, err := jsonmerge.Unmerge(filepath.Join(t.TempDir(), "absent.json"), []jsonmerge.Claim{{Path: []string{"a"}}})

	require.NoError(t, err)
	assert.False(t, got.Changed)
}

func TestApply_JSONCIsMergedAndMalformedIsRefused(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		wantErr bool
	}{
		{"line comment", "{\n  // keep\n  \"a\": 1\n}\n", false},
		{"block comment and trailing comma", "{ /* c */ \"a\": [1,2,], }", false},
		{"comment markers inside strings are data", `{"url": "http://x//y", "a": 1,}`, false},
		{"plain syntax error", `{"a": }`, true},
		{"unterminated block comment", "{\"a\": 1 /* oops }", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := filepath.Join(t.TempDir(), "opencode.json")
			require.NoError(t, os.WriteFile(path, []byte(tt.doc), 0o644))

			// Act
			_, err := jsonmerge.Apply(path, []jsonmerge.OwnedKey{{Name: "x", Value: 1}})

			// Assert
			assert.Equal(t, tt.wantErr, err != nil)
		})
	}
}

func TestApply_MembersMergeServerByServer(t *testing.T) {
	const doc = `{
  "permissions": {"allow": ["Bash"]},
  "mcpServers": {
    "mine": {"command": "mine"},
    "s1": {"command": "stale"}
  }
}
`
	tests := []struct {
		name          string
		doc           string
		servers       map[string]any
		want          string
		wantPartially bool
	}{
		{
			name:    "hand-written server survives, same name is replaced in place, new one appended",
			doc:     doc,
			servers: map[string]any{"s1": map[string]any{"command": "fresh"}, "s2": map[string]any{"command": "two"}},
			want: `{
  "permissions": {"allow": ["Bash"]},
  "mcpServers": {
    "mine": {"command": "mine"},
    "s1": {
      "command": "fresh"
    },
    "s2": {
      "command": "two"
    }
  }
}
`,
			wantPartially: true,
		},
		{
			name:          "document holding only our servers is not partially owned",
			doc:           `{"mcpServers": {"s1": {"command": "stale"}}}`,
			servers:       map[string]any{"s1": map[string]any{"command": "fresh"}},
			want:          `{"mcpServers": {"s1": {"command":"fresh"}}}`,
			wantPartially: false,
		},
		{
			name:          "no servers leaves an existing map alone",
			doc:           `{"mcpServers": {"mine": {}}}`,
			servers:       map[string]any{},
			want:          `{"mcpServers": {"mine": {}}}`,
			wantPartially: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := filepath.Join(t.TempDir(), "settings.json")
			require.NoError(t, os.WriteFile(path, []byte(tt.doc), 0o644))

			// Act
			got, err := jsonmerge.Apply(path, []jsonmerge.OwnedKey{{Name: "mcpServers", Value: tt.servers, Members: true}})

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Body)
			assert.Equal(t, tt.wantPartially, got.PartiallyOwned)
		})
	}
}

func TestApply_ArrayWithConsumerElementsIsPartiallyOwned(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "settings.json")
	require.NoError(t, os.WriteFile(doc, []byte("{\n  \"hooks\": {\"Stop\": [{\"command\": \"mine\"}]}\n}\n"), 0o644))

	tests := []struct {
		name          string
		value         []any
		elements      []any
		wantPartially bool
	}{
		{"consumer elements beside ours", []any{map[string]any{"command": "mine"}, map[string]any{"command": "ours"}},
			[]any{map[string]any{"command": "ours"}}, true},
		{"only ours", []any{map[string]any{"command": "ours"}}, []any{map[string]any{"command": "ours"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := doc
			if !tt.wantPartially {
				path = filepath.Join(dir, "fresh.json")
			}
			result, err := jsonmerge.Apply(path, []jsonmerge.OwnedKey{{Path: []string{"hooks", "Stop"}, Value: tt.value, Elements: tt.elements}})
			require.NoError(t, err)
			assert.Equal(t, tt.wantPartially, result.PartiallyOwned)
		})
	}
}

func TestAloneKeyIsTakenBackOnlyWhenNothingElseRemains(t *testing.T) {
	dir := t.TempDir()
	owned := []jsonmerge.OwnedKey{
		{Name: "version", Value: 1, Alone: true},
		{Path: []string{"hooks", "stop"}, Value: []any{map[string]any{"command": "ours"}}, Elements: []any{map[string]any{"command": "ours"}}},
	}

	t.Run("nothing else: the document goes", func(t *testing.T) {
		doc := filepath.Join(dir, "a.json")
		result, err := jsonmerge.Apply(doc, owned)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(doc, []byte(result.Body), 0o644))
		clean, err := jsonmerge.Unmerge(doc, result.Claims)
		require.NoError(t, err)
		assert.True(t, clean.Empty)
	})

	t.Run("a consumer key remains: the version stays with it", func(t *testing.T) {
		doc := filepath.Join(dir, "b.json")
		result, err := jsonmerge.Apply(doc, owned)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(doc, []byte(result.Body), 0o644))
		merged, err := jsonmerge.Apply(doc, []jsonmerge.OwnedKey{{Path: []string{"hooks", "mine"}, Value: []any{"x"}}})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(doc, []byte(merged.Body), 0o644))
		clean, err := jsonmerge.Unmerge(doc, result.Claims)
		require.NoError(t, err)
		assert.False(t, clean.Empty)
		assert.Contains(t, clean.Body, `"version": 1`)
		assert.NotContains(t, clean.Body, "ours")
	})
}

func TestClaim_ElementsAreRecordedAsDigestsNotValues(t *testing.T) {
	// Arrange
	secret := map[string]any{"name": "github", "env": map[string]any{"TOKEN": "s3cret-value"}}
	claim := jsonmerge.Claim{Path: []string{"mcp_servers"}, Elements: []any{secret}}

	// Act
	encoded, err := json.Marshal(claim)
	require.NoError(t, err)
	var back jsonmerge.Claim
	require.NoError(t, json.Unmarshal(encoded, &back))

	// Assert
	assert.NotContains(t, string(encoded), "s3cret-value")
	assert.True(t, back.OwnsElement(secret), "a digest still recognizes the element")
	assert.False(t, back.OwnsElement(map[string]any{"name": "other"}))
	assert.Nil(t, back.Elements, "no value survives the round trip")
}

func TestClaim_LegacyElementValuesAreFoldedIntoDigests(t *testing.T) {
	// Arrange: a manifest an earlier version wrote.
	legacy := `{"path":["instructions"],"elements":["AGENTS.local.md"]}`

	// Act
	var claim jsonmerge.Claim
	require.NoError(t, json.Unmarshal([]byte(legacy), &claim))
	rewritten, err := json.Marshal(claim)

	// Assert
	require.NoError(t, err)
	assert.True(t, claim.HasElements())
	assert.True(t, claim.OwnsElement("AGENTS.local.md"))
	assert.NotContains(t, string(rewritten), "AGENTS.local.md")
}
