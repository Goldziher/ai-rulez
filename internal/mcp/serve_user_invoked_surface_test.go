package mcp

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/generator"
)

const userInvokedMessage = "is user-invoked only (disable-model-invocation)"

// authoredUserInvokedProject has a served ai-rulez skill that is user-only, an open one,
// and a user-only skill kept where Claude Code looks for it (found by discovery).
func authoredUserInvokedProject(t *testing.T) string {
	t.Helper()
	root := project(t, discoverConfig, map[string]string{
		"skills/secret/SKILL.md": skillFile("secret", "A user-only authored skill", "delivery: served\ndisable-model-invocation: true\n"),
		"skills/open/SKILL.md":   skillFile("open", "An open authored skill", "delivery: served\n"),
	})
	writeFile(t, root, ".claude/skills/add-topic/SKILL.md",
		"---\nname: add-topic\ndescription: Add a new Kafka topic to a service\ndisable-model-invocation: true\n---\n\n# add-topic\n")
	writeFile(t, root, "pkg/api/.claude/skills/api-topic/SKILL.md",
		"---\nname: api-topic\ndescription: Add an api topic\ndisable-model-invocation: true\n---\n\n# api-topic\n")
	return root
}

func readURI(t *testing.T, s *sdkmcp.ClientSession, uri string) (string, error) {
	t.Helper()
	res, err := s.ReadResource(context.Background(), &sdkmcp.ReadResourceParams{URI: uri})
	if err != nil {
		return "", err //nolint:wrapcheck // the test inspects the error
	}
	return res.Contents[0].Text, nil
}

func mergedServerFor(t *testing.T, root string, serve bool) *sdkmcp.ClientSession {
	t.Helper()
	setup := &ServeSetup{WorkDir: root, DiscoverSkills: "repo", ServeUserInvoked: serve, Authoring: []Option{WithRoot(root)}}
	return connect(t, newServerFor(t, setup))
}

func TestMergedServer_AuthoringSurfaceKeepsUserInvokedSkillsFromTheModel(t *testing.T) {
	for _, serve := range []bool{false, true} {
		name := "by default"
		if serve {
			name = "with serve_user_invoked_skills"
		}
		t.Run(name, func(t *testing.T) {
			// Arrange
			session := mergedServerFor(t, authoredUserInvokedProject(t), serve)

			// Act
			readTool := call(t, session, "read_skill", map[string]any{"name": "secret"})
			readByPath := call(t, session, "read_skill", map[string]any{"name": ".claude/skills/add-topic/SKILL.md"})
			readOpen := call(t, session, "read_skill", map[string]any{"name": "open"})
			_, readResource := readURI(t, session, "ai-rulez://skills/secret")
			_, readOpenResource := readURI(t, session, "ai-rulez://skills/open")
			listed, listErr := toolJSON(t, session, "list_skills", nil)
			catalogTool, catalogErr := toolJSON(t, session, "catalog", map[string]any{"kind": "skill"})
			catalogText, err := readURI(t, session, "ai-rulez://catalog")

			// Assert
			require.NoError(t, err)
			require.False(t, listErr, "%v", listed)
			require.False(t, catalogErr, "%v", catalogTool)
			assert.False(t, readOpen.IsError, firstText(readOpen))
			require.NoError(t, readOpenResource)
			if serve {
				assert.False(t, readTool.IsError, firstText(readTool))
				assert.NoError(t, readResource)
				assert.Contains(t, listedNames(listed), "secret")
				assert.Contains(t, itemIDs(catalogTool), "secret")
				assert.Contains(t, catalogText, `"secret"`)
				assert.Contains(t, catalogText, `"add-topic"`)
				return
			}
			require.True(t, readTool.IsError)
			assert.Contains(t, firstText(readTool), "secret "+userInvokedMessage)
			require.True(t, readByPath.IsError)
			assert.Contains(t, firstText(readByPath), userInvokedMessage)
			require.Error(t, readResource)
			assert.Contains(t, readResource.Error(), "secret "+userInvokedMessage)
			assert.Equal(t, []string{"open"}, listedNames(listed))
			assert.EqualValues(t, 1, listed["count"])
			assert.Contains(t, itemIDs(catalogTool), "secret", "the audit lists every skill by name")
			assert.Contains(t, itemIDs(catalogTool), "open")
			assert.Contains(t, catalogText, `"secret"`)
			assert.NotContains(t, catalogText, "add-topic")
			assert.NotContains(t, catalogText, ".claude/skills/add-topic")
		})
	}
}

func TestAuthoringServer_StillReadsUserInvokedSkills(t *testing.T) {
	// Arrange: the plain authoring server edits the configuration; the flag only
	// keeps a skill from the model's own invocation.
	root := authoredUserInvokedProject(t)
	session := connect(t, NewServer("test", WithRoot(root)))

	// Act
	read := call(t, session, "read_skill", map[string]any{"name": "secret"})
	listed, _ := toolJSON(t, session, "list_skills", nil)
	_, resourceErr := readURI(t, session, "ai-rulez://skills/secret")

	// Assert
	assert.False(t, read.IsError, firstText(read))
	assert.ElementsMatch(t, []string{"secret", "open"}, listedNames(listed))
	assert.NoError(t, resourceErr)
}

func TestMergedServer_SkillResourcesAndToolsRefuseAHeldSkill(t *testing.T) {
	// Arrange
	session := mergedServerFor(t, authoredUserInvokedProject(t), false)

	// Act
	getByURI := call(t, session, "get_skill", map[string]any{"name": "skill://add-topic/SKILL.md"})
	getByName := call(t, session, "get_skill", map[string]any{"name": "add-topic"})
	readFile := call(t, session, "read_skill_file", map[string]any{"uri": "skill://add-topic/SKILL.md"})
	resources := call(t, session, "list_skill_resources", map[string]any{"name": "add-topic"})
	_, resourceErr := readURI(t, session, "skill://add-topic/SKILL.md")

	// Assert
	for name, res := range map[string]*sdkmcp.CallToolResult{
		"get_skill by skill:// URI": getByURI, "get_skill": getByName, "read_skill_file": readFile, "list_skill_resources": resources,
	} {
		assert.True(t, res.IsError, name)
	}
	assert.Contains(t, firstText(getByURI), "add-topic "+userInvokedMessage)
	assert.Contains(t, firstText(getByName), "add-topic "+userInvokedMessage)
	assert.Error(t, resourceErr)
}

func TestServeSetup_ScopeCountsLeaveHeldSkillsOut(t *testing.T) {
	// Arrange
	root := authoredUserInvokedProject(t)
	writeFile(t, root, "pkg/api/.claude/skills/api-open/SKILL.md", repoSkill("api-open"))

	tests := []struct {
		name  string
		serve bool
		want  string
	}{
		{name: "held skills are not counted", want: "pkg/api (1 skill)"},
		{name: "opted in", serve: true, want: "pkg/api (2 skills)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			session := mergedServerFor(t, root, tt.serve)
			catalogText, err := readURI(t, session, "ai-rulez://catalog?scope=pkg/api")

			// Assert
			require.NoError(t, err)
			assert.Contains(t, session.InitializeResult().Instructions, tt.want)
			var doc struct {
				Scopes []struct {
					Scope  string         `json:"scope"`
					Counts map[string]int `json:"counts"`
				} `json:"scopes"`
			}
			require.NoError(t, json.Unmarshal([]byte(catalogText), &doc))
			require.Len(t, doc.Scopes, 1)
			want := 1
			if tt.serve {
				want = 2
			}
			assert.Equal(t, want, doc.Scopes[0].Counts["skill"])
		})
	}
}

func TestSkillServer_ReplaceFollowsTheUserInvokedFlag(t *testing.T) {
	// Arrange
	build := func(flagged bool, names ...string) *Catalog {
		var skills []generator.ServedSkill
		for _, n := range names {
			sk := servedSkill(n, "", "The "+n+" skill", nil)
			if flagged && n == "toggle" {
				sk.Files[0].Content = []byte("---\nname: toggle\ndescription: The toggle skill\ndisable-model-invocation: true\n---\n\n# toggle\n")
			}
			skills = append(skills, sk)
		}
		cat, err := BuildCatalog("p", "claude", skills, SkillFilter{})
		require.NoError(t, err)
		return cat
	}
	// A live server (it can reload), so a client does not cache the resource listing.
	srv := NewSkillServerWith("test", build(false, "toggle", "other"), ServeOptions{Revalidate: func() *Catalog { return nil }})
	session := connect(t, srv)
	uris := func() []string {
		var out []string
		for r, err := range session.Resources(context.Background(), nil) {
			require.NoError(t, err)
			out = append(out, r.URI)
		}
		return out
	}
	listedIs := func(uri string, want bool) {
		t.Helper()
		assert.Eventually(t, func() bool { return slices.Contains(uris(), uri) == want }, 3*time.Second, 20*time.Millisecond, "%s listed: %v", uri, want)
	}
	listedIs("skill://toggle/SKILL.md", true)

	// Act and assert: the skill gains the flag on edit.
	srv.Replace(build(true, "toggle", "other"))
	listedIs("skill://toggle/SKILL.md", false)
	listedIs("skill://other/SKILL.md", true)
	_, err := readURI(t, session, "skill://toggle/SKILL.md")
	assert.Error(t, err)
	assert.Contains(t, firstText(call(t, session, "load_skill", map[string]any{"name": "toggle"})), userInvokedMessage)

	// The flagged skill is removed, then re-added without the flag.
	srv.Replace(build(true, "other"))
	listedIs("skill://toggle/SKILL.md", false)
	assert.Contains(t, firstText(call(t, session, "get_skill", map[string]any{"name": "toggle"})), "no served skill")
	srv.Replace(build(false, "toggle", "other"))
	listedIs("skill://toggle/SKILL.md", true)
	text, err := readURI(t, session, "skill://toggle/SKILL.md")
	require.NoError(t, err)
	assert.Contains(t, text, "# toggle")

	// The flag is set again.
	srv.Replace(build(true, "toggle", "other"))
	listedIs("skill://toggle/SKILL.md", false)
}

func TestServeSetup_DerivedDaemonIdentityFollowsServeUserInvoked(t *testing.T) {
	a := (&ServeSetup{Profile: "p"}).derivedDaemonIdentity()
	b := (&ServeSetup{Profile: "p", ServeUserInvoked: true}).derivedDaemonIdentity()
	assert.NotEqual(t, a, b)
}

func listedNames(out map[string]any) []string {
	var names []string
	skills, _ := out["skills"].([]any)
	for _, s := range skills {
		if m, ok := s.(map[string]any); ok {
			names = append(names, m["name"].(string))
		}
	}
	return names
}

func itemIDs(out map[string]any) []string {
	var ids []string
	items, _ := out["items"].([]any)
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			ids = append(ids, m["id"].(string))
		}
	}
	return ids
}

func TestRawArguments_NullAndAbsentArgumentsAreEmpty(t *testing.T) {
	for name, raw := range map[string]json.RawMessage{"absent": nil, "null": json.RawMessage("null"), "empty object": json.RawMessage("{}")} {
		t.Run(name, func(t *testing.T) {
			req := &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Name: "list_skills", Arguments: raw}}

			args, err := rawArguments(req)

			require.NoError(t, err)
			assert.NotNil(t, args)
		})
	}
}

func TestTools_AcceptNilArguments(t *testing.T) {
	// Arrange: a confined server writes working_directory into the arguments.
	session := connect(t, NewServer("test", WithRoot(authoredUserInvokedProject(t))))

	// Act
	res := call(t, session, "list_skills", nil)

	// Assert
	assert.False(t, res.IsError, firstText(res))
}
