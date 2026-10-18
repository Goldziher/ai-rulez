package mcp

import (
	"context"
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// projectlessRepo is a repository that keeps a skill and an instruction file
// but no ai-rulez configuration (issue 435).
func projectlessRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "skills/demo/SKILL.md", repoSkill("demo"))
	writeFile(t, root, "AGENTS.md", "# Root\n\nHello.\n")
	return root
}

func projectlessServer(t *testing.T, root string) *sdkmcp.ClientSession {
	t.Helper()
	setup := &ServeSetup{WorkDir: root, DiscoverSkills: "repo", Authoring: []Option{WithRoot(root)}}
	return connect(t, newServerFor(t, setup))
}

func resourceURIs(t *testing.T, session *sdkmcp.ClientSession) []string {
	t.Helper()
	var uris []string
	for r, err := range session.Resources(context.Background(), nil) {
		require.NoError(t, err)
		uris = append(uris, r.URI)
	}
	return uris
}

func TestProjectless_ListedResourcesCanBeRead(t *testing.T) {
	t.Parallel()
	session := projectlessServer(t, projectlessRepo(t))

	uris := resourceURIs(t, session)

	assert.Contains(t, uris, "ai-rulez://catalog")
	assert.NotContains(t, uris, "ai-rulez://config", "a configuration resource needs a configuration")
	for _, uri := range uris {
		_, err := session.ReadResource(context.Background(), &sdkmcp.ReadResourceParams{URI: uri})
		assert.NoError(t, err, "%s is listed, so reading it must work", uri)
	}
}

func TestProjectless_CatalogIsBuiltFromDiscoveredContent(t *testing.T) {
	t.Parallel()
	session := projectlessServer(t, projectlessRepo(t))

	res, err := session.ReadResource(context.Background(), &sdkmcp.ReadResourceParams{URI: "ai-rulez://catalog"})

	require.NoError(t, err)
	var doc struct {
		Items []struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		} `json:"items"`
		Scopes         []map[string]any `json:"scopes"`
		SkillDiscovery struct {
			Mode     string           `json:"mode"`
			Served   []map[string]any `json:"served"`
			Shadowed []map[string]any `json:"shadowed"`
		} `json:"skill_discovery"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.Contents[0].Text), &doc))
	require.Len(t, doc.Items, 1)
	assert.Equal(t, "skill", doc.Items[0].Kind)
	assert.Equal(t, "demo", doc.Items[0].ID)
	assert.Equal(t, "repo", doc.SkillDiscovery.Mode)
	assert.NotEmpty(t, doc.Scopes, "the instruction file of the repository is in a scope")
	assert.Contains(t, res.Contents[0].Text, `"shadowed": []`, "no shadowed skill is an empty list, not null")
}

func TestProjectless_OnlyToolsThatWorkWithoutAConfigurationAreListed(t *testing.T) {
	t.Parallel()
	session := projectlessServer(t, projectlessRepo(t))

	names := toolNames(t, session)

	for _, want := range []string{"find_skill", "load_skill", "find_context", "load_context", "search_skills", "get_version"} {
		assert.Contains(t, names, want)
	}
	for _, banned := range []string{"list_skills", "list_context", "read_config", "catalog", "create_rule", "validate_config", "doctor"} {
		assert.NotContains(t, names, banned, "%s needs a configuration", banned)
	}
	assert.Empty(t, promptNames(t, session), "every authoring prompt calls a tool that needs a configuration")
}

func TestProjectless_InstructionsDoNotPointAtMissingTools(t *testing.T) {
	t.Parallel()
	session := projectlessServer(t, projectlessRepo(t))

	got := session.InitializeResult().Instructions

	assert.Contains(t, got, "find_skill")
	assert.Contains(t, got, "no ai-rulez configuration")
	assert.NotContains(t, got, "CRUD")
	assert.NotContains(t, got, "validate_config")
}

func TestProjectless_ARootConfigFileIsAProject(t *testing.T) {
	t.Parallel()
	root := projectlessRepo(t)
	writeFile(t, root, "ai-rulez.toml", "version = \"5.0\"\nname = \"p\"\ngitignore = false\npresets = [\"claude\"]\n")
	session := projectlessServer(t, root)

	uris := resourceURIs(t, session)
	for _, uri := range uris {
		_, err := session.ReadResource(context.Background(), &sdkmcp.ReadResourceParams{URI: uri})
		assert.NoError(t, err, "%s is listed, so reading it must work", uri)
	}
	for _, name := range toolNames(t, session) {
		res, err := session.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: name, Arguments: map[string]any{}})
		if name == "init_project" || name == "generate_outputs" || name == "clean_outputs" {
			continue
		}
		require.NoError(t, err)
		if res.IsError {
			assert.NotContains(t, firstText(res), "directory not found", "tool %s is listed but fails for want of a configuration", name)
		}
	}
}
