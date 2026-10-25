package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The scope sentence of the instructions follows the catalog in force, so a live
// reload that changes which skills are served (or drops the index) is reflected.
func TestServer_ScopeNoteFollowsLiveReload(t *testing.T) {
	// Arrange
	root := authoredUserInvokedProject(t)
	writeFile(t, root, "pkg/api/.claude/skills/api-open/SKILL.md", repoSkill("api-open"))
	srv := newServerFor(t, &ServeSetup{WorkDir: root, DiscoverSkills: "repo"})
	require.Contains(t, srv.composeInstructions(), "pkg/api")
	empty, err := BuildCatalog("p", "claude", nil, SkillFilter{})
	require.NoError(t, err)

	// Act
	srv.Replace(empty)

	// Assert
	assert.NotContains(t, srv.composeInstructions(), "pkg/api")
}
