package lint

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/agentplugins"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func lintWithAgentPlugins(t *testing.T, findings []AgentPluginFinding) []Finding {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, policyFixture(""))
	gitAdd(t, root)
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	tree, err := LoadTree(root)
	require.NoError(t, err)
	rep, err := Run(cfg, tree, WithAgentPlugins(findings))
	require.NoError(t, err)
	return rep.Findings
}

func TestAgentPluginFindingsMapToTheAR9OBlock(t *testing.T) {
	cases := []struct {
		lib      string
		sev      agentplugins.Severity
		wantCode string
		wantSev  Severity
	}{
		{agentplugins.CodeManifestInvalid, agentplugins.SeverityError, CodeAgentPluginManifest, SeverityError},
		{agentplugins.CodeSkillInvalid, agentplugins.SeverityError, CodeAgentPluginSkill, SeverityError},
		{agentplugins.CodeServerInvalid, agentplugins.SeverityError, CodeAgentPluginMCP, SeverityError},
		{agentplugins.CodePlaceholder, agentplugins.SeverityError, CodeAgentPluginPlaceholder, SeverityError},
		{agentplugins.CodeFieldIgnored, agentplugins.SeverityWarning, CodeAgentPluginDropped, SeverityWarning},
		{agentplugins.CodePathEscape, agentplugins.SeverityError, CodeAgentPluginUnsafe, SeverityError},
	}
	for _, tc := range cases {
		t.Run(tc.lib, func(t *testing.T) {
			got := lintWithAgentPlugins(t, []AgentPluginFinding{{
				File: "plugin.json", Finding: agentplugins.Finding{Code: tc.lib, Severity: tc.sev, Path: "mcp.json#/mcpServers/x", Message: "boom"},
			}})
			var hit *Finding
			for i := range got {
				if got[i].Code == tc.wantCode {
					hit = &got[i]
				}
			}
			require.NotNil(t, hit, "%v", got)
			assert.Equal(t, tc.wantSev, hit.Severity)
			assert.Contains(t, hit.Message, "mcp.json#/mcpServers/x")
			assert.Contains(t, hit.Message, "boom")
		})
	}
}

func TestAgentPluginInfoFindingsAreNotReported(t *testing.T) {
	got := lintWithAgentPlugins(t, []AgentPluginFinding{{
		File: "mcp.json", Finding: agentplugins.Finding{Code: agentplugins.CodePlaceholderRewritten, Severity: agentplugins.SeverityInfo, Message: "rewritten"},
	}})
	for _, f := range got {
		assert.NotEqual(t, CodeAgentPluginPlaceholder, f.Code)
	}
}

func TestAgentPluginCodesAreExplainable(t *testing.T) {
	for _, code := range []string{CodeAgentPluginManifest, CodeAgentPluginSkill, CodeAgentPluginMCP,
		CodeAgentPluginPlaceholder, CodeAgentPluginDropped, CodeAgentPluginUnsafe} {
		ex, ok := Explain(code)
		require.True(t, ok, code)
		assert.NotEmpty(t, ex.Why, code)
	}
}

func TestAgentPluginCodesMatchThePackage(t *testing.T) {
	assert.Equal(t, agentplugins.RuleManifest, CodeAgentPluginManifest)
	assert.Equal(t, agentplugins.RuleSkill, CodeAgentPluginSkill)
	assert.Equal(t, agentplugins.RuleMCP, CodeAgentPluginMCP)
	assert.Equal(t, agentplugins.RulePlaceholder, CodeAgentPluginPlaceholder)
	assert.Equal(t, agentplugins.RuleDropped, CodeAgentPluginDropped)
	assert.Equal(t, agentplugins.RuleUnsafe, CodeAgentPluginUnsafe)
}
