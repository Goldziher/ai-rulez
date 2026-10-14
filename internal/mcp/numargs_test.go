package mcp

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
)

func TestIntArg(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		want    int
		wantErr string
	}{
		{"absent takes the default", map[string]any{}, 7, ""},
		{"whole number", map[string]any{"offset": float64(42)}, 42, ""},
		{"zero", map[string]any{"offset": float64(0)}, 0, ""},
		{"an integral float from JSON", map[string]any{"offset": 3.0}, 3, ""},
		{"too large", map[string]any{"offset": 1e30}, 0, "offset"},
		{"just past the bound", map[string]any{"offset": float64(maxIntArg) + 1}, 0, "offset"},
		{"NaN", map[string]any{"offset": math.NaN()}, 0, "finite"},
		{"infinity", map[string]any{"offset": math.Inf(1)}, 0, "finite"},
		{"negative", map[string]any{"offset": -1.0}, 0, "offset"},
		{"fractional", map[string]any{"offset": 1.5}, 0, "whole"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			req := handlers.NewToolRequest(nil, tt.args)

			// Act
			got, err := intArg(req, "offset", 7)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSkillFileToolsRejectOutOfRangeNumbers(t *testing.T) {
	t.Parallel()
	cat, _ := bundleCatalog(t)
	p, _ := startSkillServerWith(t, cat, ServeOptions{})
	tests := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"read_skill_file", map[string]any{"uri": "skill://served/assets/b.bin", "offset": 1e30}, "offset"},
		{"read_skill_file", map[string]any{"uri": "skill://served/assets/b.bin", "length": 1e30}, "length"},
		{"list_skill_resources", map[string]any{"name": "served", "offset": 1e30}, "offset"},
		{"list_skill_resources", map[string]any{"name": "served", "offset": -1}, "offset"},
	}
	for _, tt := range tests {
		t.Run(tt.tool+"/"+tt.want, func(t *testing.T) {
			// Act
			_, isErr, text := callTool(t, p, tt.tool, tt.args)

			// Assert
			assert.True(t, isErr, text)
			assert.Contains(t, text, tt.want)
		})
	}
}
