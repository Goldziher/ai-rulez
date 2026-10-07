package emit

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ard"
)

func ardFixture() *ARDInput {
	return &ARDInput{
		Publisher: "acme.test", Namespace: "conventions",
		UpdatedAt: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC),
		Skills: []ARDSkill{
			{
				Name: "deploy", Description: "Deploy the service safely.", Version: "1.0.0",
				Source: ".ai-rulez/skills/deploy/SKILL.md", Body: skillMD,
				Keywords: []string{"release", "ops"}, Triggers: []string{"deploy to production"},
				Queries:     []string{"how do I deploy the api"},
				EvalPrompts: []string{"ship the staging build", "roll back the last release"},
			},
			{
				Name: "review", Description: "Review a change.", Source: ".ai-rulez/skills/review/SKILL.md", Body: "# Review\n",
				Triggers: []string{"review my diff", "check this pull request"},
			},
		},
		Servers: []ARDServer{
			{Name: "docs", Description: "Search the docs.", Transport: "http", URL: "https://docs.acme.test/mcp"},
			{Name: "local-tools", Description: "Local helper.", Transport: "stdio", Command: "npx", Args: []string{"-y", "tools"}},
		},
	}
}

func ardInput() Input { return fixtureFor("ard") }

func TestARD_EntriesForSkillsServersAndPlugins(t *testing.T) {
	files, findings, err := ardEmitter{}.Emit(ardInput())

	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "ard.json", files[0].Path)
	var doc struct {
		Entries []map[string]any `json:"entries"`
	}
	require.NoError(t, json.Unmarshal(files[0].Data, &doc))
	byID := map[string]map[string]any{}
	for _, e := range doc.Entries {
		byID[e["identifier"].(string)] = e
	}
	require.Len(t, byID, 5)

	deploy := byID["urn:air:acme.test:conventions:deploy"]
	assert.Equal(t, "application/ai-skill+md", deploy["type"])
	assert.Equal(t, "https://raw.githubusercontent.com/acme/skills/v1.4.0/.ai-rulez/skills/deploy/SKILL.md", deploy["url"])
	assert.Equal(t, []any{"how do I deploy the api", "ship the staging build", "roll back the last release", "deploy to production"}, deploy["representativeQueries"])
	assert.Equal(t, "2026-10-07T12:00:00Z", deploy["updatedAt"])
	assert.NotContains(t, deploy, "data")

	docs := byID["urn:air:acme.test:conventions:docs"]
	assert.Equal(t, "application/mcp-server-card+json", docs["type"])
	assert.NotContains(t, docs, "url")
	assert.Equal(t, map[string]any{"name": "docs", "description": "Search the docs.",
		"transport": map[string]any{"type": "streamable-http", "url": "https://docs.acme.test/mcp"}}, docs["data"])
	local := byID["urn:air:acme.test:conventions:local-tools"]["data"].(map[string]any)["transport"]
	assert.Equal(t, map[string]any{"type": "stdio", "command": "npx", "args": []any{"-y", "tools"}}, local)

	plugin := byID["urn:air:acme.test:conventions:acme-conventions"]
	assert.Equal(t, ard.DefaultMediaTypePlugin, plugin["type"])
	assert.Equal(t, "https://github.com/acme/skills/releases/download/v1.4.0/acme-conventions-1.4.0.tar.gz", plugin["url"])
	assert.Equal(t, "1.4.0", plugin["version"])
	assert.Equal(t, []any{"agent-plugins", "conventions"}, plugin["tags"])
	assert.Equal(t, []any{"development"}, plugin["capabilities"])

	var noQueries []string
	for _, f := range findings {
		noQueries = append(noQueries, f.Message)
	}
	assert.Len(t, noQueries, 2, "the two servers have no representative queries: %v", noQueries)
	assert.Contains(t, noQueries[0], ard.CodeQueries)
}

func TestARD_ConfiguredQueriesOverrideDerivedOnes(t *testing.T) {
	in := ardInput()
	in.ARD.Queries = map[string][]string{"docs": {"search the acme docs", "find the api reference"}}

	files, findings, err := ardEmitter{}.Emit(in)

	require.NoError(t, err)
	assert.Contains(t, string(files[0].Data), "search the acme docs")
	assert.Len(t, findings, 1)
}

func TestARD_BaseURLHostsTheSkillFiles(t *testing.T) {
	in := ardInput()
	in.ARD.BaseURL = "https://acme.test/ard/"

	files, _, err := ardEmitter{}.Emit(in)

	require.NoError(t, err)
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	assert.Equal(t, []string{"ard.json", "skills/deploy/SKILL.md", "skills/review/SKILL.md"}, paths)
	assert.Equal(t, skillMD, string(files[1].Data))
	assert.Contains(t, string(files[0].Data), `"url": "https://acme.test/ard/skills/deploy/SKILL.md"`)
}

func TestARD_PluginTypeOverride(t *testing.T) {
	in := ardInput()
	in.ARD.PluginType = "application/vnd.example.plugin+json"

	files, _, err := ardEmitter{}.Emit(in)

	require.NoError(t, err)
	assert.Contains(t, string(files[0].Data), "application/vnd.example.plugin+json")
}

func TestARD_Refusals(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Input)
		want   string
	}{
		{"not configured", func(in *Input) { in.ARD = nil }, "[ard]"},
		{"bad publisher", func(in *Input) { in.ARD.Publisher = "localhost" }, ard.CodeIdentifier},
		{"no tag", func(in *Input) { in.Tag = "" }, "tag"},
		{"no repo", func(in *Input) { in.Repo = "" }, "base_url"},
		{"other forge needs a base url", func(in *Input) { in.Repo = "git.acme.test/acme/skills" }, "base_url"},
		{"credentials in a server url", func(in *Input) { in.ARD.Servers[0].URL = "https://u:p@docs.acme.test/mcp" }, "credentials"},
		{"http server url", func(in *Input) { in.ARD.Servers[0].URL = "http://docs.acme.test/mcp" }, "https"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := ardInput()
			tt.mutate(&in)

			_, _, err := ardEmitter{}.Emit(in)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestARD_NoClockInOutput(t *testing.T) {
	in := ardInput()
	in.ARD.UpdatedAt = time.Time{}

	files, _, err := ardEmitter{}.Emit(in)

	require.NoError(t, err)
	assert.NotContains(t, string(files[0].Data), "updatedAt")
}

func TestARDModel_ForLint(t *testing.T) {
	in := ardInput()
	in.Repo, in.Tag = "", ""
	in.ARD.BaseURL = "https://acme.test/ard"

	m, files, err := ARDModel(in)

	require.NoError(t, err)
	assert.Len(t, m.Resources, 5)
	assert.Len(t, files, 2)
	findings, err := ard.Check(m)
	require.NoError(t, err)
	assert.False(t, ard.HasErrors(findings))
}

func TestARD_OutputValidatesAgainstThePinnedSchemaAndIsStable(t *testing.T) {
	a, _, err := ardEmitter{}.Emit(ardInput())
	require.NoError(t, err)
	b, _, err := ardEmitter{}.Emit(ardInput())
	require.NoError(t, err)
	assert.Equal(t, a, b)

	found, err := ard.Validate(a[0].Data)

	require.NoError(t, err)
	for _, f := range found {
		assert.NotEqual(t, ard.SeverityError, f.Severity, "%+v", f)
	}
}
