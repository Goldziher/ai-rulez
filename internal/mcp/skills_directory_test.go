package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/generator"
)

func TestSkillServer_ResourcesDirectoryRead(t *testing.T) {
	// Arrange
	served := append(testServed(), servedSkill("deep", "", "Nested files", nil,
		generator.ServedSkillFile{RelPath: "templates/a.md", Content: []byte("a")},
		generator.ServedSkillFile{RelPath: "templates/regional/b.md", Content: []byte("b")}))
	cat, err := BuildCatalog("p", "claude", served, SkillFilter{})
	require.NoError(t, err)
	p := startSkillServer(t, cat)

	type child struct{ uri, mime string }
	tests := []struct {
		name    string
		uri     string
		want    []child
		wantErr bool
	}{
		{"skill root", "skill://pdf-processing", []child{
			{"skill://pdf-processing/SKILL.md", "text/markdown"},
			{"skill://pdf-processing/references", "inode/directory"},
			{"skill://pdf-processing/scripts", "inode/directory"},
		}, false},
		{"subdirectory", "skill://deep/templates", []child{
			{"skill://deep/templates/a.md", "text/markdown"},
			{"skill://deep/templates/regional", "inode/directory"},
		}, false},
		{"unknown directory", "skill://deep/nope", nil, true},
		{"a file is not a directory", "skill://deep/SKILL.md", nil, true},
		{"unknown skill", "skill://nope", nil, true},
		{"trailing slash is not a directory resource", "skill://deep/templates/", nil, true},
		{"path traversal", "skill://deep/templates/../..", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			resp := p.call("resources/directory/read", map[string]any{"uri": tt.uri})

			// Assert
			if tt.wantErr {
				require.NotNil(t, resp["error"], "%v", resp)
				assert.InDelta(t, -32602, resp["error"].(map[string]any)["code"], 0)
				return
			}
			require.Nil(t, resp["error"], "%v", resp)
			result := resp["result"].(map[string]any)
			assert.Equal(t, "complete", result["resultType"])
			var got []child
			for _, r := range result["resources"].([]any) {
				m := r.(map[string]any)
				got = append(got, child{m["uri"].(string), m["mimeType"].(string)})
			}
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("the capability declares directoryRead", func(t *testing.T) {
		resp := p.call("initialize", map[string]any{
			"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
			"clientInfo": map[string]any{"name": "t", "version": "1"},
		})
		ext := resp["result"].(map[string]any)["capabilities"].(map[string]any)["extensions"].(map[string]any)
		assert.Equal(t, true, ext[SkillsExtensionID].(map[string]any)["directoryRead"])
	})
}
