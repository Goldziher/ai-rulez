package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contentHandlers struct {
	create, read, update, remove, list func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error)
}

func TestContentHandlers_LocalRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		h        contentHandlers
		rel      string // below .ai-rulez/local
		sharedIn string // where the shared tree would hold it
	}{
		{"rule", contentHandlers{CreateRuleHandler, ReadRuleHandler, UpdateRuleHandler, DeleteRuleHandler, ListRulesHandler},
			"rules/item.md", "rules/item.md"},
		{"context", contentHandlers{CreateContextHandler, ReadContextHandler, UpdateContextHandler, DeleteContextHandler, ListContextsHandler},
			"context/item.md", "context/item.md"},
		{"skill", contentHandlers{CreateSkillHandler, ReadSkillHandler, UpdateSkillHandler, DeleteSkillHandler, ListSkillsHandler},
			"skills/item/SKILL.md", "skills/item/SKILL.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeMinimalConfig(t, dir)
			cfgDir := filepath.Join(dir, ".ai-rulez")
			args := func(extra map[string]any) *ToolRequest {
				base := map[string]any{"working_directory": dir, "name": "item", "local": true}
				for k, v := range extra {
					base[k] = v
				}
				return newRequestWithArgs(base)
			}
			ctx := context.Background()
			localFile := filepath.Join(cfgDir, "local", filepath.FromSlash(tt.rel))

			// Act + Assert: create
			res, err := tt.h.create(ctx, args(map[string]any{"content": "ORIGINAL"}))
			require.NoError(t, err)
			require.False(t, res.IsError, textOf(t, res))
			assert.FileExists(t, localFile)
			assert.NoFileExists(t, filepath.Join(cfgDir, filepath.FromSlash(tt.sharedIn)))

			// read
			res, err = tt.h.read(ctx, args(nil))
			require.NoError(t, err)
			require.False(t, res.IsError, textOf(t, res))
			assert.Contains(t, textOf(t, res), "ORIGINAL")

			// list: the local view has it, the shared view does not
			res, err = tt.h.list(ctx, args(nil))
			require.NoError(t, err)
			assert.Contains(t, textOf(t, res), "item")
			res, err = tt.h.list(ctx, newRequestWithArgs(map[string]any{"working_directory": dir}))
			require.NoError(t, err)
			assert.NotContains(t, textOf(t, res), `"item"`)

			// update
			res, err = tt.h.update(ctx, args(map[string]any{"content": "CHANGED"}))
			require.NoError(t, err)
			require.False(t, res.IsError, textOf(t, res))
			data, err := os.ReadFile(localFile)
			require.NoError(t, err)
			assert.Contains(t, string(data), "CHANGED")

			// shared read does not see local content
			res, err = tt.h.read(ctx, newRequestWithArgs(map[string]any{"working_directory": dir, "name": "item"}))
			require.NoError(t, err)
			assert.True(t, res.IsError)

			// delete
			res, err = tt.h.remove(ctx, args(nil))
			require.NoError(t, err)
			require.False(t, res.IsError, textOf(t, res))
			assert.NoFileExists(t, localFile)
		})
	}
}

func TestContentHandlers_LocalWithDomain(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeMinimalConfig(t, dir)

	// Act
	res, err := CreateRuleHandler(context.Background(), newRequestWithArgs(map[string]any{
		"working_directory": dir, "name": "r", "domain": "scratch", "local": true, "content": "BODY",
	}))

	// Assert
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(t, res))
	assert.FileExists(t, filepath.Join(dir, ".ai-rulez", "local", "domains", "scratch", "rules", "r.md"))
	assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez", "domains", "scratch"))
}
