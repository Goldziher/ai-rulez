package handlers

import (
	"context"
	"path/filepath"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type handlerFunc func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error)

func TestMCPHandlersMapEveryKindToTheAgentsTree(t *testing.T) {
	kinds := []struct {
		name   string
		path   string // below .agents
		create handlerFunc
		list   handlerFunc
		read   handlerFunc
		remove handlerFunc
	}{
		{"rule", "rules/item.md", CreateRuleHandler, ListRulesHandler, ReadRuleHandler, DeleteRuleHandler},
		{"context", "context/item.md", CreateContextHandler, ListContextsHandler, ReadContextHandler, DeleteContextHandler},
		{"skill", "skills/item/SKILL.md", CreateSkillHandler, ListSkillsHandler, ReadSkillHandler, DeleteSkillHandler},
		{"agent", "agents/item.md", CreateAgentHandler, ListAgentsHandler, ReadAgentHandler, DeleteAgentHandler},
		{"command", "commands/item.md", CreateCommandHandler, ListCommandsHandler, ReadCommandHandler, DeleteCommandHandler},
		{"check", "checks/item.md", CreateCheckHandler, ListChecksHandler, ReadCheckHandler, DeleteCheckHandler},
	}
	for _, k := range kinds {
		t.Run(k.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeAgentsMCPFile(t, dir, ".agents/ai-rulez.toml", agentsMCPConfig)
			args := map[string]any{"working_directory": dir, "name": "item", "description": "An item", "content": "Body\n"}
			ctx := context.Background()

			// Act
			created, createErr := k.create(ctx, newRequestWithArgs(args))
			listed, listErr := k.list(ctx, newRequestWithArgs(map[string]any{"working_directory": dir}))
			read, readErr := k.read(ctx, newRequestWithArgs(args))

			// Assert
			require.NoError(t, createErr)
			require.False(t, created.IsError, textOf(t, created))
			assert.FileExists(t, filepath.Join(dir, ".agents", filepath.FromSlash(k.path)))
			assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez"))
			require.NoError(t, listErr)
			require.False(t, listed.IsError, textOf(t, listed))
			assert.Contains(t, textOf(t, listed), "item")
			require.NoError(t, readErr)
			require.False(t, read.IsError, textOf(t, read))
			assert.Contains(t, textOf(t, read), "Body")

			// Act: delete
			removed, removeErr := k.remove(ctx, newRequestWithArgs(map[string]any{"working_directory": dir, "name": "item", "confirm": true}))

			// Assert
			require.NoError(t, removeErr)
			require.False(t, removed.IsError, textOf(t, removed))
			assert.NoFileExists(t, filepath.Join(dir, ".agents", filepath.FromSlash(k.path)))
		})
	}
}
