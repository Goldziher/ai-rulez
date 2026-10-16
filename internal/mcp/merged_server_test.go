package mcp

import (
	"context"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// toolNames lists the tool names a connected client sees.
func toolNames(t *testing.T, session *sdkmcp.ClientSession) []string {
	t.Helper()
	var names []string
	for tool, err := range session.Tools(context.Background(), nil) {
		require.NoError(t, err)
		names = append(names, tool.Name)
	}
	return names
}

// TestMergedServer_ServesSkillsAndAuthoringFromOneServer: `mcp --serve-skills`
// now registers both surfaces under one name, so a project needs one MCP entry.
func TestMergedServer_ServesSkillsAndAuthoringFromOneServer(t *testing.T) {
	t.Parallel()
	srv := NewMergedServer("test", loadCatalog(t), ServeOptions{}, WithAnyDirectory())
	session := connect(t, srv)

	assert.Equal(t, "ai-rulez", session.InitializeResult().ServerInfo.Name,
		"one server named ai-rulez, not ai-rulez-skills")

	names := toolNames(t, session)
	for _, want := range []string{"find_skill", "load_skill", "search_skills", "validate_config", "create_skill", "generate_outputs"} {
		assert.Contains(t, names, want, "the merged server registers both surfaces")
	}
}

// TestMergedServer_ReadOnlyRegistersNoWritingTool: --read-only keeps the
// read-only config and report tools and drops every tool that writes.
func TestMergedServer_ReadOnlyRegistersNoWritingTool(t *testing.T) {
	t.Parallel()
	srv := NewMergedServer("test", loadCatalog(t), ServeOptions{}, WithAnyDirectory(), WithReadOnly())
	session := connect(t, srv)

	names := toolNames(t, session)
	for _, banned := range []string{"create_skill", "update_skill", "delete_skill", "generate_outputs", "clean_outputs", "update_config"} {
		assert.NotContains(t, names, banned, "a read-only merged server must not register %s", banned)
	}
	for _, want := range []string{"find_skill", "load_skill", "validate_config", "read_config", "doctor", "lock_status"} {
		assert.Contains(t, names, want, "the read-only tools stay")
	}
}

// TestMergedServer_ResourcesFromBothSurfaces: the merged server answers reads
// for a skill:// resource and an ai-rulez:// resource on one session, and still
// reports an unknown URI as not found.
func TestMergedServer_ResourcesFromBothSurfaces(t *testing.T) {
	t.Parallel()
	root := telemetryProject(t)
	srv := NewMergedServer("test", loadCatalog(t), ServeOptions{}, WithRoot(root))
	session := connect(t, srv)
	read := func(uri string) (*sdkmcp.ReadResourceResult, error) {
		return session.ReadResource(context.Background(), &sdkmcp.ReadResourceParams{URI: uri})
	}

	var uris []string
	for r, err := range session.Resources(context.Background(), nil) {
		require.NoError(t, err)
		uris = append(uris, r.URI)
	}
	assert.Contains(t, uris, "ai-rulez://config")
	assert.Contains(t, uris, "skill://git-workflow/SKILL.md")

	skill, err := read("skill://git-workflow/SKILL.md")
	require.NoError(t, err)
	assert.NotEmpty(t, skill.Contents[0].Text)
	cfg, err := read("ai-rulez://config")
	require.NoError(t, err)
	assert.Contains(t, cfg.Contents[0].Text, `"t"`)

	_, err = read("skill://nope/SKILL.md")
	assert.Error(t, err, "an unknown skill URI is not found")
	_, err = read("ai-rulez://rules/nope")
	assert.Error(t, err, "an unknown item URI is not found")
}

// TestAuthoringServer_ReadOnlyDropsWritingTools: `mcp --read-only` is the
// authoring server without the tools that change a project.
func TestAuthoringServer_ReadOnlyDropsWritingTools(t *testing.T) {
	t.Parallel()
	session := connect(t, NewServer("test", WithAnyDirectory(), WithReadOnly()))

	names := toolNames(t, session)
	assert.NotContains(t, names, "create_rule")
	assert.NotContains(t, names, "generate_outputs")
	assert.Contains(t, names, "read_config")
	assert.Contains(t, names, "validate_config")
}

// The judge tool sends file content to a model provider and spends money, so no read-only server
// registers it, and a server that is not read-only does.
func TestJudgeToolIsMissingFromReadOnlyServers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		srv       *Server
		wantJudge bool
	}{
		{"authoring, read-only", NewServer("test", WithAnyDirectory(), WithReadOnly()), false},
		{"merged, read-only", NewMergedServer("test", loadCatalog(t), ServeOptions{}, WithAnyDirectory(), WithReadOnly()), false},
		{"authoring", NewServer("test", WithAnyDirectory()), true},
		{"merged", NewMergedServer("test", loadCatalog(t), ServeOptions{}, WithAnyDirectory()), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			session := connect(t, tc.srv)

			// Act
			names := toolNames(t, session)

			// Assert
			if tc.wantJudge {
				assert.Contains(t, names, "judge")
			} else {
				assert.NotContains(t, names, "judge")
				assert.Contains(t, names, "validate_config", "the read-only tools stay")
			}
		})
	}
}
