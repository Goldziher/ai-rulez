package mcp

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func userInvokedRolesProject(t *testing.T) string {
	t.Helper()
	root := authoredUserInvokedProject(t)
	writeFile(t, root, ".ai-rulez/config.toml", discoverConfig+"\n[[roles]]\nname = \"engineer\"\n")
	return root
}

func TestMergedServer_ReportToolsDoNotNameUserInvokedSkills(t *testing.T) {
	tools := []string{"list_roles", "catalog", "token_report", "cost_report", "sbom", "doctor", "validate_config", "scan_content"}
	for _, serve := range []bool{false, true} {
		t.Run(fmt.Sprintf("serve_user_invoked_skills=%v", serve), func(t *testing.T) {
			// Arrange
			session := mergedServerFor(t, userInvokedRolesProject(t), serve)

			for _, tool := range tools {
				// Act
				text := firstText(call(t, session, tool, map[string]any{}))

				// Assert
				if !serve {
					assert.NotContains(t, text, "secret", tool)
					assert.NotContains(t, text, "add-topic", tool)
				}
				if serve && (tool == "list_roles" || tool == "cost_report" || tool == "sbom") {
					assert.Contains(t, text, "secret", tool)
				}
			}
		})
	}
}

func TestMergedServer_RoleTotalsLeaveUserInvokedSkillsOut(t *testing.T) {
	// Arrange
	session := mergedServerFor(t, userInvokedRolesProject(t), false)

	// Act
	out, isErr := toolJSON(t, session, "list_roles", map[string]any{})

	// Assert
	require.False(t, isErr, "%v", out)
	roles, _ := out["roles"].([]any)
	require.Len(t, roles, 1)
	role, _ := roles[0].(map[string]any)
	totals, _ := role["totals"].(map[string]any)
	byKind, _ := totals["by_kind"].(map[string]any)
	assert.Len(t, role["items"], 1)
	assert.EqualValues(t, 1, totals["items"])
	assert.EqualValues(t, 1, totals["served_skills"])
	assert.EqualValues(t, 1, byKind["skill"])
}

func TestResolveRole_ReturnsTheRoleObject(t *testing.T) {
	// Arrange
	session := mergedServerFor(t, userInvokedRolesProject(t), true)

	// Act
	out, isErr := toolJSON(t, session, "resolve_role", map[string]any{"role": "engineer"})

	// Assert
	require.False(t, isErr, "%v", out)
	role, ok := out["role"].(map[string]any)
	require.True(t, ok, "role is an object: %v", out["role"])
	assert.Equal(t, "engineer", role["name"])
}

func TestResolveRole_LeavesUserInvokedSkillsOut(t *testing.T) {
	// Arrange
	session := mergedServerFor(t, userInvokedRolesProject(t), false)

	// Act
	text := firstText(call(t, session, "resolve_role", map[string]any{"role": "engineer"}))

	// Assert
	assert.Contains(t, text, "engineer")
	assert.NotContains(t, text, "secret")
}
