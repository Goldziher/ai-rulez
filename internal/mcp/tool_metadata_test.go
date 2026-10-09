package mcp

import (
	"context"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readOnlyTools are the tools that never write; every other tool must not
// claim ReadOnlyHint.
var readOnlyTools = map[string]bool{
	"validate_config": true, "doctor": true, "run_verifiers": true,
	"get_version": true, "show_builtin": true,
	"list_domains": true, "read_rule": true, "list_rules": true,
	"read_check": true, "list_checks": true,
	"read_context": true, "list_context": true,
	"read_skill": true, "list_skills": true,
	"list_includes": true, "list_installed_skills": true,
	"read_config": true, "list_profiles": true,
	"list_roles": true, "resolve_role": true, "lock_status": true, "catalog": true,
	"find_skill": true, "load_skill": true, "list_skill_resources": true,
	"search_skills": true, "get_skill": true, "read_skill_file": true,
}

func listAllTools(t *testing.T, srv *Server) []*sdkmcp.Tool {
	t.Helper()
	session := connect(t, srv)
	var out []*sdkmcp.Tool
	for tool, err := range session.Tools(context.Background(), nil) {
		require.NoError(t, err)
		out = append(out, tool)
	}
	return out
}

func TestEveryToolCarriesMetadata(t *testing.T) {
	t.Parallel()
	authoring := listAllTools(t, NewServer("test"))
	serving := listAllTools(t, NewSkillServerWith("test", loadCatalog(t), ServeOptions{}))
	tools := append(append([]*sdkmcp.Tool{}, authoring...), serving...)
	require.Len(t, tools, 53, "tool count changed: update the metadata expectations deliberately")

	for _, tool := range tools {
		t.Run(tool.Name, func(t *testing.T) {
			assert.NotEmpty(t, tool.Title, "Title")
			assert.NotEqual(t, tool.Name, tool.Title, "Title is a human label, not the snake_case name")
			assert.NotEmpty(t, tool.Description)

			require.NotNil(t, tool.Annotations, "annotations")
			a := tool.Annotations
			require.NotNil(t, a.DestructiveHint, "destructiveHint is set explicitly")
			require.NotNil(t, a.OpenWorldHint, "openWorldHint is set explicitly")
			assert.False(t, *a.OpenWorldHint, "ai-rulez tools act on the local project tree only")

			assert.NotNil(t, tool.InputSchema, "input schema")
			schema, ok := tool.OutputSchema.(map[string]any)
			require.True(t, ok, "output schema is an object, got %T", tool.OutputSchema)
			assert.Equal(t, "object", schema["type"])

			if readOnlyTools[tool.Name] {
				assert.True(t, a.ReadOnlyHint, "%s only reads", tool.Name)
				assert.False(t, *a.DestructiveHint)
			} else {
				assert.False(t, a.ReadOnlyHint, "%s writes", tool.Name)
			}
			if strings.HasPrefix(tool.Name, "delete_") || strings.HasPrefix(tool.Name, "remove_") || tool.Name == "clean_outputs" {
				assert.True(t, *a.DestructiveHint, "%s removes content", tool.Name)
			}
		})
	}
}

func TestToolsHaveStrictInputSchemas(t *testing.T) {
	t.Parallel()
	for _, tool := range listAllTools(t, NewServer("test")) {
		schema, ok := tool.InputSchema.(map[string]any)
		require.True(t, ok, tool.Name)
		assert.Equal(t, false, schema["additionalProperties"], "%s rejects unknown arguments", tool.Name)
	}
}
