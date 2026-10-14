package mcp

import (
	"context"
	"math"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
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

func TestLimitArg(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		want    int
		wantErr string
	}{
		{"absent takes the default", map[string]any{}, 7, ""},
		{"whole number", map[string]any{"limit": 3.0}, 3, ""},
		{"zero takes the default", map[string]any{"limit": 0.0}, 7, ""},
		{"negative takes the default", map[string]any{"limit": -1.0}, 7, ""},
		{"too large is rejected", map[string]any{"limit": 1e30}, 0, "limit"},
		{"NaN is rejected", map[string]any{"limit": math.NaN()}, 0, "finite"},
		{"infinity is rejected", map[string]any{"limit": math.Inf(1)}, 0, "finite"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			req := handlers.NewToolRequest(nil, tt.args)

			// Act
			got, err := limitArg(req, "limit", 7)

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

// The handlers are called directly: over the wire the tool's input schema
// already refuses most of these values before the handler runs.
func TestLimitHandlersRejectOutOfRangeNumbers(t *testing.T) {
	t.Parallel()
	cat, _ := bundleCatalog(t)
	_, srv := startSkillServerWith(t, cat, ServeOptions{})
	tests := []struct {
		name    string
		handler func(context.Context, *handlers.ToolRequest) (*sdkmcp.CallToolResult, error)
		args    map[string]any
		want    string
	}{
		{"find_skill 1e30", srv.findSkillHandler, map[string]any{"task": "serve", "limit": 1e30}, "limit"},
		{"find_skill NaN", srv.findSkillHandler, map[string]any{"task": "serve", "limit": math.NaN()}, "limit"},
		{"search_skills 1e30", srv.searchSkillsHandler, map[string]any{"query": "served", "limit": 1e30}, "limit"},
		{"search_skills NaN", srv.searchSkillsHandler, map[string]any{"query": "served", "limit": math.NaN()}, "limit"},
		{"load_skill 1e30", srv.loadSkillHandler, map[string]any{"name": "served", "budget_bytes": 1e30}, "budget_bytes"},
		{"load_skill NaN", srv.loadSkillHandler, map[string]any{"name": "served", "budget_bytes": math.NaN()}, "budget_bytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			res, err := tt.handler(context.Background(), handlers.NewToolRequest(nil, tt.args))

			// Assert
			require.NoError(t, err)
			require.True(t, res.IsError)
			assert.Contains(t, res.Content[0].(*sdkmcp.TextContent).Text, tt.want)
		})
	}
}

func TestLimitHandlersTreatNegativeAsDefault(t *testing.T) {
	t.Parallel()
	cat, _ := bundleCatalog(t)
	_, srv := startSkillServerWith(t, cat, ServeOptions{})
	for _, args := range []map[string]any{{"task": "serve", "limit": -1.0}, {"task": "serve", "limit": 0.0}} {
		// Act
		res, err := srv.findSkillHandler(context.Background(), handlers.NewToolRequest(nil, args))

		// Assert
		require.NoError(t, err)
		assert.False(t, res.IsError, "%v", res.Content[0])
	}
	res, err := srv.loadSkillHandler(context.Background(), handlers.NewToolRequest(nil, map[string]any{"name": "served", "budget_bytes": -5.0}))
	require.NoError(t, err)
	assert.False(t, res.IsError, "%v", res.Content[0])
}
