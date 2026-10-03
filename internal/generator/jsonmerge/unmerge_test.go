package jsonmerge_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
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
			want:  []jsonmerge.Claim{{Path: []string{"effort"}}},
		},
		{
			name: "members claim each server by name, sorted",
			owned: []jsonmerge.OwnedKey{{
				Path: []string{"mcp", "servers"}, Members: true,
				Value: map[string]any{"b": 1, "a": 2},
			}},
			want: []jsonmerge.Claim{{Path: []string{"mcp", "servers", "a"}}, {Path: []string{"mcp", "servers", "b"}}},
		},
		{
			name:  "empty members map claims the key",
			owned: []jsonmerge.OwnedKey{{Name: "mcpServers", Members: true, Value: map[string]any{}}},
			want:  []jsonmerge.Claim{{Path: []string{"mcpServers"}}},
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
			want:        "{\n  \"mcpServers\": {\n    \"mine\": {\"command\": \"x\"}\n  }\n}\n",
			wantChanged: true,
		},
		{
			name:        "document with nothing else left is empty",
			doc:         `{"mcpServers": {"h": {}}}`,
			claims:      []jsonmerge.Claim{{Path: []string{"mcpServers", "h"}}},
			want:        "{}\n",
			wantChanged: true,
			wantEmpty:   true,
		},
		{
			name:        "elements are removed and the user's stay",
			doc:         `{"instructions": ["mine.md", "AGENTS.local.md"]}`,
			claims:      []jsonmerge.Claim{{Path: []string{"instructions"}, Elements: []any{"AGENTS.local.md", "./AGENTS.local.md"}}},
			want:        "{\n  \"instructions\": [\n    \"mine.md\"\n  ]\n}\n",
			wantChanged: true,
		},
		{
			name:        "an emptied array drops the key and its empty parents",
			doc:         `{"context": {"fileName": ["GEMINI.local.md"]}, "theme": "dark"}`,
			claims:      []jsonmerge.Claim{{Path: []string{"context", "fileName"}, Elements: []any{"GEMINI.local.md"}}},
			want:        "{\n  \"theme\": \"dark\"\n}\n",
			wantChanged: true,
		},
		{
			name:        "equals guard keeps a value that is not ours",
			doc:         `{"$schema": "https://example.com/mine.json"}`,
			claims:      []jsonmerge.Claim{{Path: []string{"$schema"}, Equals: "https://opencode.ai/config.json"}},
			want:        "",
			wantChanged: false,
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
			want:        "{}\n",
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

func TestApplyReportsCommentsDistinctly(t *testing.T) {
	tests := []struct {
		name     string
		doc      string
		wantSoft bool
	}{
		{"line comment", "{\n  // keep\n  \"a\": 1\n}\n", true},
		{"block comment and trailing comma", "{ /* c */ \"a\": [1,2,], }", true},
		{"comment markers inside strings are data", `{"url": "http://x//y", "a": 1,}`, true},
		{"plain syntax error", `{"a": }`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := filepath.Join(t.TempDir(), "opencode.json")
			require.NoError(t, os.WriteFile(path, []byte(tt.doc), 0o644))

			// Act
			_, err := jsonmerge.Apply(path, []jsonmerge.OwnedKey{{Name: "x", Value: 1}})

			// Assert
			require.Error(t, err)
			assert.Equal(t, tt.wantSoft, errors.Is(err, jsonmerge.ErrNotStrictJSON))
		})
	}
}
