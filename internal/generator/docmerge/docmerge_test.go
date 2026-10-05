package docmerge_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/generator/docmerge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatFromPath(t *testing.T) {
	tests := []struct {
		path   string
		want   docmerge.Format
		wantOK bool
	}{
		{".claude/settings.json", docmerge.FormatJSON, true},
		{"kilo.jsonc", docmerge.FormatJSONC, true},
		{".codex/config.toml", docmerge.FormatTOML, true},
		{".rovodev/config.yml", docmerge.FormatYAML, true},
		{".takt/config.YAML", docmerge.FormatYAML, true},
		{"README.md", "", false},
		{"Makefile", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			// Act
			got, ok := docmerge.FormatFromPath(tt.path)

			// Assert
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

// TestApplyUnmerge runs the same ownership round trip through every format: the
// pre-existing document comes back byte for byte after Unmerge, and the claims
// survive a trip through the JSON manifest.
func TestApplyUnmerge(t *testing.T) {
	owned := []docmerge.OwnedKey{{
		Path:    []string{"servers"},
		Value:   map[string]any{"gen": map[string]any{"command": "x"}},
		Members: true,
	}}
	tests := []struct {
		name     string
		file     string
		format   docmerge.Format
		original string
	}{
		{"json", "settings.json", docmerge.FormatJSON, "{\n  \"model\": \"opus\"\n}\n"},
		{"jsonc", "settings.json", docmerge.FormatJSONC, "{\n  // mine\n  \"model\": \"opus\", // pinned\n}\n"},
		{"toml", "config.toml", docmerge.FormatTOML, "# mine\nmodel = \"opus\" # pinned\n"},
		{"yaml", "config.yaml", docmerge.FormatYAML, "# mine\nmodel: opus # pinned\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := filepath.Join(t.TempDir(), tt.file)
			require.NoError(t, os.WriteFile(path, []byte(tt.original), 0o600))

			// Act
			applied, err := docmerge.Apply(path, tt.format, owned)
			require.NoError(t, err)
			require.NotEqual(t, tt.original, applied.Body)
			assert.True(t, applied.PartiallyOwned)

			manifest, err := json.Marshal(applied.Claims)
			require.NoError(t, err)
			var claims []docmerge.Claim
			require.NoError(t, json.Unmarshal(manifest, &claims))
			unmerged, err := docmerge.UnmergeDocument(path, tt.format, applied.Body, claims)

			// Assert
			require.NoError(t, err)
			assert.True(t, unmerged.Changed)
			assert.False(t, unmerged.Empty)
			assert.Equal(t, tt.original, unmerged.Body)
		})
	}
}

func TestApply_NewDocumentIsWhollyOwned(t *testing.T) {
	owned := []docmerge.OwnedKey{{Name: "servers", Value: map[string]any{"gen": map[string]any{"command": "x"}}}}
	for _, format := range []docmerge.Format{docmerge.FormatJSON, docmerge.FormatTOML, docmerge.FormatYAML} {
		t.Run(string(format), func(t *testing.T) {
			// Act
			result, err := docmerge.Apply(filepath.Join(t.TempDir(), "missing"), format, owned)

			// Assert
			require.NoError(t, err)
			assert.NotEmpty(t, result.Body)
			assert.False(t, result.PartiallyOwned)
			assert.Len(t, result.Claims, 1)
		})
	}
}

func TestUnsupportedFormat(t *testing.T) {
	// Act
	_, applyErr := docmerge.Apply("x", "xml", nil)
	_, unmergeErr := docmerge.Unmerge("x", "", nil)
	_, docErr := docmerge.UnmergeDocument("x", "ini", "", nil)

	// Assert
	require.Error(t, applyErr)
	require.Error(t, unmergeErr)
	require.Error(t, docErr)
	assert.False(t, docmerge.Format("xml").Valid())
	assert.True(t, docmerge.FormatYAML.Valid())
}
