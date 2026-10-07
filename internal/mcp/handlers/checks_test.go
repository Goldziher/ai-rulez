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

func TestCheckHandlers_RoundTrip(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeMinimalConfig(t, dir)
	ctx := context.Background()
	args := func(extra map[string]any) *ToolRequest {
		base := map[string]any{"working_directory": dir, "name": "security"}
		for k, v := range extra {
			base[k] = v
		}
		return newRequestWithArgs(base)
	}
	file := filepath.Join(dir, ".ai-rulez", "checks", "security.md")

	// Act + Assert: create builds frontmatter from the structured fields
	res, err := CreateCheckHandler(ctx, args(map[string]any{
		"content": "Flag injection.", "description": "Security", "severity": "high",
		"tools": []any{"Read", "Grep"}, "targets": []any{"cursor"},
	}))
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(t, res))
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(data), "severity: high")
	assert.Contains(t, string(data), "Flag injection.")

	res, err = ReadCheckHandler(ctx, args(nil))
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(t, res))
	assert.Contains(t, textOf(t, res), "Flag injection.")

	res, err = UpdateCheckHandler(ctx, args(map[string]any{"content": "Updated body.", "severity": "low"}))
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(t, res))
	data, err = os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Updated body.")
	assert.Contains(t, string(data), "severity: low")

	res, err = ListChecksHandler(ctx, args(nil))
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(t, res))
	assert.Contains(t, textOf(t, res), "security")

	res, err = DeleteCheckHandler(ctx, args(nil))
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(t, res))
	assert.NoFileExists(t, file)
}

func wrap(h func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error)) func(*ToolRequest) (bool, error) {
	return func(req *ToolRequest) (bool, error) {
		res, err := h(context.Background(), req)
		if err != nil {
			return false, err
		}
		return res.IsError, nil
	}
}

func TestCheckHandlers_RejectBadInput(t *testing.T) {
	dir := t.TempDir()
	writeMinimalConfig(t, dir)

	tests := []struct {
		name string
		call func(*ToolRequest) (bool, error)
		args map[string]any
	}{
		{"traversal on create", wrap(CreateCheckHandler), map[string]any{"name": "../escape", "content": "x"}},
		{"traversal on read", wrap(ReadCheckHandler), map[string]any{"name": "../../etc/passwd"}},
		{"traversal on delete", wrap(DeleteCheckHandler), map[string]any{"name": "a/b"}},
		{"bad severity", wrap(CreateCheckHandler), map[string]any{"name": "ok", "content": "x", "severity": "urgent"}},
		{"missing on read", wrap(ReadCheckHandler), map[string]any{"name": "absent"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := map[string]any{"working_directory": dir}
			for k, v := range tt.args {
				base[k] = v
			}
			isErr, err := tt.call(newRequestWithArgs(base))
			require.NoError(t, err)
			assert.True(t, isErr)
		})
	}
	assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez", "checks"))
}

func TestUpdateCheckHandler_MergesAndRejectsAnEmptyUpdate(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeMinimalConfig(t, dir)
	ctx := context.Background()
	args := func(extra map[string]any) *ToolRequest {
		base := map[string]any{"working_directory": dir, "name": "security"}
		for k, v := range extra {
			base[k] = v
		}
		return newRequestWithArgs(base)
	}
	file := filepath.Join(dir, ".ai-rulez", "checks", "security.md")
	res, err := CreateCheckHandler(ctx, args(map[string]any{
		"content": "Original body.", "description": "Original: description", "severity": "high",
		"tools": []any{"Read"}, "targets": []any{"cursor"},
	}))
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(t, res))

	// Act + Assert: nothing to change is an error and the file stays as it was.
	before, err := os.ReadFile(file)
	require.NoError(t, err)
	res, err = UpdateCheckHandler(ctx, args(nil))
	require.NoError(t, err)
	assert.True(t, res.IsError, "an update with neither content nor a field must be rejected")
	after, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))

	// Content without frontmatter replaces the body only.
	res, err = UpdateCheckHandler(ctx, args(map[string]any{"content": "New body."}))
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(t, res))
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(data), "New body.")
	assert.Contains(t, string(data), "severity: high")
	assert.Contains(t, string(data), "Original: description")
	assert.Contains(t, string(data), "cursor")
	assert.NotContains(t, string(data), "priority")

	// A field alone changes the frontmatter and keeps the body.
	res, err = UpdateCheckHandler(ctx, args(map[string]any{"severity": "low"}))
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(t, res))
	data, err = os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(data), "severity: low")
	assert.Contains(t, string(data), "New body.")
}

func TestCreateCheckHandler_ValidatesTargetsAndKeepsFrontmatterContent(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeMinimalConfig(t, dir)
	ctx := context.Background()

	// A misspelled preset selects no output at all, so it is rejected.
	res, err := CreateCheckHandler(ctx, newRequestWithArgs(map[string]any{
		"working_directory": dir, "name": "typo", "content": "x", "targets": []any{"curser"},
	}))
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.NoFileExists(t, filepath.Join(dir, ".ai-rulez", "checks", "typo.md"))

	// A full file keeps its frontmatter and takes the fields over it.
	res, err = CreateCheckHandler(ctx, newRequestWithArgs(map[string]any{
		"working_directory": dir, "name": "full",
		"content":  "---\nseverity: low\ncustom: 1\n---\nBody.\n",
		"severity": "critical",
	}))
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(t, res))
	data, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "checks", "full.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "severity: critical")
	assert.Contains(t, string(data), "custom: 1")
	assert.Contains(t, string(data), "Body.")

	// Only a description: the template body stays and the description is quoted safely.
	res, err = CreateCheckHandler(ctx, newRequestWithArgs(map[string]any{
		"working_directory": dir, "name": "described", "description": "Flags: injection # not a comment", "severity": "high",
	}))
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(t, res))
	data, err = os.ReadFile(filepath.Join(dir, ".ai-rulez", "checks", "described.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "severity: high")
	assert.Contains(t, string(data), "Describe what the reviewer should look for.")
	assert.Contains(t, string(data), "Flags: injection # not a comment")
}
