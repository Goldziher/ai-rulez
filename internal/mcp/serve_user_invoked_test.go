package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func userInvokedProject(t *testing.T) string {
	t.Helper()
	root := project(t, discoverConfig, nil)
	for rel, body := range map[string]string{
		".claude/skills/add-topic/SKILL.md":    "---\nname: add-topic\ndescription: Add a new Kafka topic to a service\ndisable-model-invocation: true\n---\n\n# add-topic\n",
		".claude/skills/quoted-topic/SKILL.md": "---\nname: quoted-topic\ndescription: Add a Kafka topic, quoted flag\ndisable-model-invocation: \"true\"\n---\n\n# quoted\n",
		".claude/skills/open-topic/SKILL.md":   "---\nname: open-topic\ndescription: Add a Kafka topic, flag off\ndisable-model-invocation: false\n---\n\n# open\n",
		".claude/skills/plain-topic/SKILL.md":  "---\nname: plain-topic\ndescription: Add a Kafka topic, no flag\n---\n\n# plain\n",
	} {
		writeFile(t, root, rel, body)
	}
	return root
}

func TestCatalog_WithoutUserInvoked(t *testing.T) {
	tests := []struct {
		name     string
		front    map[string]any
		wantHeld bool
	}{
		{name: "true", front: map[string]any{"disable-model-invocation": true}, wantHeld: true},
		{name: "the string true, any case", front: map[string]any{"disable-model-invocation": " True "}, wantHeld: true},
		{name: "false", front: map[string]any{"disable-model-invocation": false}},
		{name: "the string false", front: map[string]any{"disable-model-invocation": "false"}},
		{name: "absent", front: map[string]any{}},
		{name: "the underscore misspelling is not Claude Code's key", front: map[string]any{"disable_model_invocation": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			skill := &CatalogSkill{Name: "s", URI: "skill://s/SKILL.md", Frontmatter: tt.front, Files: []CatalogFile{{URI: "skill://s/SKILL.md"}}}
			cat := &Catalog{
				skills: []*CatalogSkill{skill},
				byName: map[string]*CatalogSkill{"s": skill}, byURI: map[string]*CatalogSkill{skill.URI: skill},
				byFile: map[string]*CatalogFile{"skill://s/SKILL.md": &skill.Files[0]},
			}

			// Act
			got := cat.withoutUserInvoked()

			// Assert
			assert.Equal(t, tt.wantHeld, got.UserInvoked("s"))
			_, found := got.Lookup("s")
			assert.Equal(t, !tt.wantHeld, found)
			assert.Len(t, cat.Skills(), 1, "the receiver is not modified")
			if tt.wantHeld {
				assert.Empty(t, got.Skills())
				_, file := got.File("skill://s/SKILL.md")
				assert.False(t, file)
			}
		})
	}
}

func TestSkillServer_LeavesUserInvokedSkillsOutOfTheModelView(t *testing.T) {
	tests := []struct {
		name  string
		serve bool
		want  []string
	}{
		{name: "by default", want: []string{"open-topic", "plain-topic"}},
		{name: "with serve_user_invoked_skills", serve: true, want: []string{"add-topic", "quoted-topic", "open-topic", "plain-topic"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			srv := newServerFor(t, &ServeSetup{WorkDir: userInvokedProject(t), DiscoverSkills: "repo", ServeUserInvoked: tt.serve})
			session := connect(t, srv)

			// Act
			found, findErr := toolJSON(t, session, "find_skill", map[string]any{"task": "add a Kafka topic", "limit": 20})
			searched, searchErr := toolJSON(t, session, "search_skills", map[string]any{"query": "Kafka topic", "limit": 20})
			listed, err := session.ListResources(context.Background(), nil)

			// Assert
			require.NoError(t, err)
			require.False(t, findErr, "%v", found)
			require.False(t, searchErr, "%v", searched)
			assert.ElementsMatch(t, tt.want, resultNames(t, found))
			assert.ElementsMatch(t, tt.want, resultNames(t, searched))
			var uris []string
			for _, r := range listed.Resources {
				uris = append(uris, r.URI)
			}
			assert.Equal(t, tt.serve, contains(uris, "skill://add-topic/SKILL.md"))
			assert.Contains(t, uris, "skill://plain-topic/SKILL.md")
		})
	}
}

func TestSkillServer_RefusesToLoadAUserInvokedSkill(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		args  map[string]any
		serve bool
		want  string
	}{
		{name: "load_skill", tool: "load_skill", args: map[string]any{"name": "add-topic"}, want: "add-topic is user-invoked only (disable-model-invocation)"},
		{name: "load_skill by URI", tool: "load_skill", args: map[string]any{"name": "skill://quoted-topic/SKILL.md"}, want: "quoted-topic is user-invoked only (disable-model-invocation)"},
		{name: "get_skill", tool: "get_skill", args: map[string]any{"name": "add-topic"}, want: "add-topic is user-invoked only (disable-model-invocation)"},
		{name: "an unflagged skill still loads", tool: "load_skill", args: map[string]any{"name": "open-topic"}},
		{name: "opted in", tool: "load_skill", args: map[string]any{"name": "add-topic"}, serve: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			srv := newServerFor(t, &ServeSetup{WorkDir: userInvokedProject(t), DiscoverSkills: "repo", ServeUserInvoked: tt.serve})
			session := connect(t, srv)

			// Act
			res := call(t, session, tt.tool, tt.args)

			// Assert
			if tt.want == "" {
				assert.False(t, res.IsError, firstText(res))
				return
			}
			require.True(t, res.IsError)
			assert.Contains(t, firstText(res), tt.want)
		})
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
