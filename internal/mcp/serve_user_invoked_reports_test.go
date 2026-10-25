package mcp

import (
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func connectMerged(t *testing.T, root string, serve bool, deny ...string) *sdkmcp.ClientSession {
	t.Helper()
	setup := &ServeSetup{WorkDir: root, DiscoverSkills: "repo", ServeUserInvoked: serve, Filter: SkillFilter{Deny: deny}, Authoring: []Option{WithRoot(root)}}
	return connect(t, newServerFor(t, setup))
}

func userInvokedRolesProject(t *testing.T) string {
	t.Helper()
	root := authoredUserInvokedProject(t)
	writeFile(t, root, ".ai-rulez/config.toml", discoverConfig+"\n[[roles]]\nname = \"engineer\"\n")
	return root
}

// The governance and audit tools report on the whole configuration, so they list a
// user-only skill by name with its size, as they did before: what they carry is no
// content the model could invoke, and narrowing them would make an audit lie.
func TestAuditTools_ListUserInvokedSkillsByName(t *testing.T) {
	session := mergedServerFor(t, userInvokedRolesProject(t), false)

	for _, tool := range []string{"list_roles", "catalog", "cost_report"} {
		text := firstText(call(t, session, tool, map[string]any{}))
		assert.Contains(t, text, "secret", tool)
	}
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

func TestCreateSkill_ExistingNameDoesNotRevealThePath(t *testing.T) {
	session := mergedServerFor(t, authoredUserInvokedProject(t), false)

	res := call(t, session, "create_skill", map[string]any{"name": "secret", "content": "---\ndescription: A replacement skill for secrets\n---\n\nbody\n"})

	require.True(t, res.IsError)
	assert.Contains(t, firstText(res), "already exists")
	assert.NotContains(t, firstText(res), ".ai-rulez")
}

// A held skill is identified by its own frontmatter, so a user-only skill the
// catalog never admitted (here: denied by --deny) is still kept out of the
// authoring reads and the listing, and a same-named skill elsewhere is not.
func TestAuthoringReads_HeldByFrontmatterNotByCatalog(t *testing.T) {
	root := authoredUserInvokedProject(t)
	session := connectMerged(t, root, false, "secret")

	read := call(t, session, "read_skill", map[string]any{"name": "secret"})
	listed, _ := toolJSON(t, session, "list_skills", map[string]any{})
	open := call(t, session, "read_skill", map[string]any{"name": "open"})

	require.True(t, read.IsError)
	assert.Contains(t, firstText(read), userInvokedMessage)
	assert.ElementsMatch(t, []string{"open"}, listedNames(listed))
	assert.False(t, open.IsError, firstText(open))
}
