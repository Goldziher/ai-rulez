package handlers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type handlerFunc func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error)

func decodeResult(t *testing.T, res *sdkmcp.CallToolResult) map[string]any {
	t.Helper()
	require.False(t, res.IsError, textOf(t, res))
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(textOf(t, res)), &out))
	return out
}

func TestMCPHandlersMapEveryKindToTheAgentsTree(t *testing.T) {
	kinds := []struct {
		name    string
		path    string // below .agents
		listKey string // key of the list in the list result
		want    string // exact content of the created file
		create  handlerFunc
		list    handlerFunc
		read    handlerFunc
		remove  handlerFunc
	}{
		{"rule", "rules/item.md", "rules",
			"---\ntype: Decision\ntitle: Item\nx-ai-rulez:\n  kind: rule\n  id: item\n  metadata:\n    priority: medium\n---\n\nBody\n",
			CreateRuleHandler, ListRulesHandler, ReadRuleHandler, DeleteRuleHandler},
		{"context", "context/item.md", "contexts",
			"---\ntype: Concept\ntitle: Item\nx-ai-rulez:\n  kind: context\n  id: item\n  metadata:\n    priority: medium\n---\n\nBody\n",
			CreateContextHandler, ListContextsHandler, ReadContextHandler, DeleteContextHandler},
		{"skill", "skills/item/SKILL.md", "skills",
			"---\ntype: Playbook\ntitle: Item\ndescription: An item\nx-ai-rulez:\n  kind: skill\n  id: item\n  metadata:\n    priority: medium\n    name: item\n---\n\nBody\n",
			CreateSkillHandler, ListSkillsHandler, ReadSkillHandler, DeleteSkillHandler},
		{"agent", "agents/item.md", "agents",
			"---\ntype: Reference\ntitle: Item\nx-ai-rulez:\n  kind: agent\n  id: item\n---\n\nBody\n",
			CreateAgentHandler, ListAgentsHandler, ReadAgentHandler, DeleteAgentHandler},
		{"command", "commands/item.md", "commands",
			"---\ntype: Reference\ntitle: Item\nx-ai-rulez:\n  kind: command\n  id: item\n---\n\nBody\n",
			CreateCommandHandler, ListCommandsHandler, ReadCommandHandler, DeleteCommandHandler},
		{"check", "checks/item.md", "checks",
			"---\ntype: Reference\ntitle: Item\ndescription: An item\nx-ai-rulez:\n  kind: check\n  id: item\n---\n\nBody\n",
			CreateCheckHandler, ListChecksHandler, ReadCheckHandler, DeleteCheckHandler},
	}
	for _, k := range kinds {
		t.Run(k.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeAgentsMCPFile(t, dir, ".agents/ai-rulez.toml", agentsMCPConfig)
			args := map[string]any{"working_directory": dir, "name": "item", "description": "An item", "content": "Body\n"}
			ctx := context.Background()
			wantPath := filepath.Join(dir, ".agents", filepath.FromSlash(k.path))

			// Act
			created, createErr := k.create(ctx, newRequestWithArgs(args))
			listed, listErr := k.list(ctx, newRequestWithArgs(map[string]any{"working_directory": dir}))
			read, readErr := k.read(ctx, newRequestWithArgs(args))

			// Assert
			require.NoError(t, createErr)
			require.NoError(t, listErr)
			require.NoError(t, readErr)
			createdOut := decodeResult(t, created)
			assert.Equal(t, "item", createdOut["name"])
			assert.Equal(t, filepath.ToSlash(wantPath), filepath.ToSlash(createdOut["path"].(string)))
			onDisk, fileErr := os.ReadFile(wantPath)
			require.NoError(t, fileErr)
			assert.Equal(t, k.want, string(onDisk))
			assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez"))

			listOut := decodeResult(t, listed)
			assert.EqualValues(t, 1, listOut["count"])
			items, ok := listOut[k.listKey].([]any)
			require.True(t, ok, "list result has %q: %v", k.listKey, listOut)
			require.Len(t, items, 1)
			item := items[0].(map[string]any)
			assert.Equal(t, "item", item["name"])
			assert.Equal(t, filepath.ToSlash(wantPath), filepath.ToSlash(item["path"].(string)))

			readOut := decodeResult(t, read)
			assert.Equal(t, k.want, readOut["content"])
			assert.Equal(t, filepath.ToSlash(wantPath), filepath.ToSlash(readOut["path"].(string)))

			// Act: delete
			removed, removeErr := k.remove(ctx, newRequestWithArgs(map[string]any{"working_directory": dir, "name": "item", "confirm": true}))

			// Assert
			require.NoError(t, removeErr)
			decodeResult(t, removed)
			assert.NoFileExists(t, wantPath)
		})
	}
}
