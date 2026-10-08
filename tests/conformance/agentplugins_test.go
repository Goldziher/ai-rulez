package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/agentplugins"
)

// upstreamNamePattern is the plugin name pattern of the official schema. Its
// lookahead is not valid RE2, so no Go validator compiles it; the test strips
// the keyword and applies the rule below instead.
const upstreamNamePattern = `^(?!.*(?:--|\.\.))[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`

var nameChars = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

// validPluginName is upstreamNamePattern written without the lookahead.
func validPluginName(name string) bool {
	return nameChars.MatchString(name) && !regexp.MustCompile(`--|\.\.`).MatchString(name)
}

type schemaPair struct{ plugin, mcp *jsonschema.Schema }

func officialPluginSchema(t *testing.T, spec string) *schemaPair {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(schemaFile(t, "agent-plugins/"+spec+"/plugin.schema.json"), &doc))
	name := doc["properties"].(map[string]any)["name"].(map[string]any)
	require.Equal(t, upstreamNamePattern, name["pattern"], "the upstream name pattern changed; review validPluginName")
	delete(name, "pattern")
	stripped, err := json.Marshal(doc)
	require.NoError(t, err)
	id, _ := doc["$id"].(string)
	require.NotEmpty(t, id)
	mcpDoc := schemaFile(t, "agent-plugins/"+spec+"/mcp.schema.json")
	var mcpMeta map[string]any
	require.NoError(t, json.Unmarshal(mcpDoc, &mcpMeta))
	mcpID, _ := mcpMeta["$id"].(string)
	require.NotEmpty(t, mcpID)
	return &schemaPair{
		plugin: compileDocs(t, id, map[string][]byte{id: stripped}),
		mcp:    compileDocs(t, mcpID, map[string][]byte{mcpID: mcpDoc}),
	}
}

// checkPluginDir validates a plugin directory the way a conformant client reads it.
func checkPluginDir(t *testing.T, dir, spec string) {
	t.Helper()
	schemas := officialPluginSchema(t, spec)

	pluginJSON := read(t, dir, "plugin.json")
	requireValid(t, schemas.plugin, pluginJSON)
	var manifest struct {
		Schema string `json:"$schema"`
		Name   string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(pluginJSON, &manifest))
	assert.Equal(t, "https://agent-plugins.org/schemas/"+spec+"/plugin.schema.json", manifest.Schema)
	assert.True(t, validPluginName(manifest.Name), "plugin name %q breaks the official name pattern", manifest.Name)

	if _, err := os.Stat(filepath.Join(dir, "mcp.json")); err == nil {
		mcpJSON := read(t, dir, "mcp.json")
		requireValid(t, schemas.mcp, mcpJSON)
		assert.Contains(t, string(mcpJSON), "https://agent-plugins.org/schemas/"+spec+"/mcp.schema.json")
	}

	skills, err := filepath.Glob(filepath.Join(dir, "skills", "*", "SKILL.md"))
	require.NoError(t, err)
	for _, skill := range skills {
		checkSkill(t, filepath.Base(filepath.Dir(skill)), read(t, skill))
	}

	res := agentplugins.Validate(os.DirFS(dir))
	assert.False(t, res.Rejected)
	assert.Empty(t, res.Findings)
	assert.Equal(t, spec, res.Spec)
}

func TestAgentPluginsGoldenConformsToTheOfficialSchemas(t *testing.T) {
	for _, spec := range agentplugins.Specs {
		t.Run(spec, func(t *testing.T) {
			checkPluginDir(t, filepath.Join(repoRoot, "internal", "agentplugins", "testdata", "golden", spec), spec)
		})
	}
}

func TestAgentPluginsGeneratedByTheCLIConformToTheOfficialSchemas(t *testing.T) {
	for _, spec := range agentplugins.Specs {
		t.Run(spec, func(t *testing.T) {
			// Arrange
			dir := project(t, map[string]string{
				".ai-rulez/config.toml": `version = "5.0"
name = "conf"
presets = ["claude"]

[plugin]
name = "acme.tools"
version = "1.2.0"
description = "Acme tooling."
runtimes = ["agent-plugins"]
spec = "` + spec + `"

[[mcp_servers]]
name = "tools"
command = "npx"
args = ["-y", "@acme/mcp"]

[[mcp_servers]]
name = "remote"
transport = "http"
url = "https://mcp.acme.example/mcp"
`,
				".ai-rulez/skills/deploy/SKILL.md": "---\nname: deploy\ndescription: Ship it.\n---\nDeploy.\n",
			})

			// Act
			run(t, dir, "generate", "--yes", "--plugin")

			// Assert
			checkPluginDir(t, dir, spec)
		})
	}
}

func TestAgentPluginsSchemasRejectBrokenManifests(t *testing.T) {
	schemas := officialPluginSchema(t, agentplugins.DefaultSpec)
	requireInvalid(t, schemas.plugin, `{"name":"x"}`)
	requireInvalid(t, schemas.plugin, `{"name":"Bad Name","version":"1.0.0","description":"d"}`)
	requireInvalid(t, schemas.mcp, `{"mcpServers":{"a":{"type":"stdio"}}}`)
	assert.False(t, validPluginName("a--b"))
	assert.False(t, validPluginName("a..b"))
	assert.True(t, validPluginName("acme.tools-2"))
}
