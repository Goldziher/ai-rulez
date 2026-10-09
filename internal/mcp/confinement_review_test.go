package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestContentToolsUnderRootRefuseSymlinks(t *testing.T) {
	t.Parallel()
	dir := telemetryProject(t)
	cfg := filepath.Join(dir, ".ai-rulez")
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("TOPSECRET"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(cfg, "domains"), 0o750))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(cfg, "domains", "evil"))
	testutil.SymlinkOrSkip(t, secret, filepath.Join(cfg, "rules", "leak.md"))
	session := connect(t, NewServer("test", WithRoot(dir)))

	tests := map[string]map[string]any{
		"create_rule": {"name": "pwn", "domain": "evil", "content": "x"},
		"update_rule": {"name": "pwn", "domain": "evil", "content": "x"},
		"delete_rule": {"name": "leak"},
		"read_rule":   {"name": "leak"},
	}
	for tool, args := range tests {
		t.Run(tool, func(t *testing.T) {
			args["working_directory"] = dir
			res := call(t, session, tool, args)

			assert.True(t, res.IsError, textOfResult(t, res))
			assert.NotContains(t, textOfResult(t, res), "TOPSECRET")
		})
	}
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "only the secret is in the outside directory")
	assert.FileExists(t, secret)
}

func TestUpdateRuleKeepsPriorityAndTargets(t *testing.T) {
	t.Parallel()
	dir := telemetryProject(t)
	session := connect(t, NewServer("test", WithRoot(dir)))

	res := call(t, session, "create_rule", map[string]any{"working_directory": dir, "name": "keep", "content": "first", "priority": "critical", "targets": []any{"claude", "cursor"}})
	require.False(t, res.IsError, textOfResult(t, res))
	res = call(t, session, "update_rule", map[string]any{"working_directory": dir, "name": "keep", "content": "second"})
	require.False(t, res.IsError, textOfResult(t, res))

	data, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "rules", "keep.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "priority: critical")
	assert.Contains(t, string(data), "cursor")
	assert.Contains(t, string(data), "second")
}

func TestListToolsUseLowerCaseKeysAndATypedSchema(t *testing.T) {
	t.Parallel()
	dir := telemetryProject(t)
	session := connect(t, NewServer("test", WithRoot(dir)))

	res := call(t, session, "list_rules", map[string]any{"working_directory": dir})
	require.False(t, res.IsError, textOfResult(t, res))
	rules, ok := structured(t, res)["rules"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, rules)
	item, ok := rules[0].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, item, "name")
	assert.Contains(t, item, "path")
	assert.NotContains(t, item, "Name")
	assert.NotContains(t, item, "Path")

	tools, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	for _, tool := range tools.Tools {
		if tool.Name != "list_rules" {
			continue
		}
		schema, err := json.Marshal(tool.OutputSchema)
		require.NoError(t, err)
		assert.Contains(t, string(schema), `"priority"`, "the rule items have a described shape")
	}
}
