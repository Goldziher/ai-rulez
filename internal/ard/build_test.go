package ard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var release = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

// fixtureModel is a project with two skills, two MCP servers and a plugin,
// listed out of order with untidy tags so Build has to normalise them.
func fixtureModel() Model {
	return Model{
		Publisher: "example.com",
		Namespace: "ai-rulez",
		UpdatedAt: release,
		Resources: []Resource{
			{
				Kind: KindPlugin, Name: "governance", DisplayName: "Governance plugin",
				Description: "Rules, skills and MCP servers for AI governance.",
				Version:     "5.0.0",
				URL:         "https://github.com/example/governance/releases/download/v5.0.0/plugin.json",
				Tags:        []string{"governance", "plugin"},
				RepresentativeQueries: []string{
					"install the governance plugin", "set up AI coding rules for my repo",
				},
			},
			{
				Kind: KindSkill, Name: "release-notes", Description: "Write release notes from merged PRs.",
				Version: "1.2.0", UpdatedAt: time.Date(2026, 8, 30, 9, 30, 0, 0, time.FixedZone("CEST", 2*3600)),
				URL:  "https://example.com/skills/release-notes/SKILL.md",
				Tags: []string{"release", " docs ", "release"},
				RepresentativeQueries: []string{
					"draft release notes for v2.1", "summarize the merged pull requests since the last tag",
					"write a changelog entry",
				},
				Capabilities: []string{"ReleaseNotes"},
			},
			{
				Kind: KindMCPServer, Name: "ai-rulez", DisplayName: "ai-rulez MCP server",
				Description: "Read and edit ai-rulez configuration.",
				Data:        map[string]any{"name": "ai-rulez", "transport": map[string]any{"type": "stdio"}},
				RepresentativeQueries: []string{
					"add a rule to my ai-rulez config", "regenerate the assistant files",
				},
				Capabilities: []string{"generate_outputs", "create_rule"},
			},
			{
				Kind: KindSkill, Name: "code-review", Description: "Review a diff for bugs.",
				URL:                   "https://example.com/skills/code-review/SKILL.md",
				RepresentativeQueries: []string{"review my changes", "check this PR for bugs"},
			},
			{
				Kind: KindMCPServer, Name: "search", Description: "Web search.",
				URL:                   "https://example.com/mcp/search/server-card.json",
				RepresentativeQueries: []string{"search the web for the go 1.27 release notes", "find docs about json schema"},
			},
		},
	}
}

func TestBuildGolden(t *testing.T) {
	// Act
	out, findings, err := Build(fixtureModel())

	// Assert
	require.NoError(t, err)
	assert.Empty(t, findings)
	golden := filepath.Join("testdata", "golden", "ard.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(golden), 0o755))
		require.NoError(t, os.WriteFile(golden, out, 0o600))
	}
	want, err := os.ReadFile(golden)
	require.NoError(t, err, "missing golden; UPDATE_GOLDEN=1 go test ./internal/ard")
	assert.Equal(t, string(want), string(out))
}

func TestBuildOutputValidatesAgainstTheSchema(t *testing.T) {
	// Arrange
	out, _, err := Build(fixtureModel())
	require.NoError(t, err)

	// Act
	findings, err := Validate(out)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestBuildIsDeterministic(t *testing.T) {
	// Arrange
	a := fixtureModel()
	b := fixtureModel()
	for i, j := 0, len(b.Resources)-1; i < j; i, j = i+1, j-1 {
		b.Resources[i], b.Resources[j] = b.Resources[j], b.Resources[i]
	}

	// Act
	outA, _, errA := Build(a)
	outB, _, errB := Build(b)

	// Assert
	require.NoError(t, errA)
	require.NoError(t, errB)
	assert.Equal(t, string(outA), string(outB))
}

func TestBuildUpdatedAt(t *testing.T) {
	tests := []struct {
		name     string
		model    time.Time
		resource time.Time
		want     string
	}{
		{name: "resource wins", model: release, resource: release.Add(time.Hour), want: `"updatedAt": "2026-09-01T13:00:00Z"`},
		{name: "model default", model: release, want: `"updatedAt": "2026-09-01T12:00:00Z"`},
		{name: "omitted when unknown", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			m := oneSkill()
			m.UpdatedAt = tt.model
			m.Resources[0].UpdatedAt = tt.resource

			// Act
			out, _, err := Build(m)

			// Assert
			require.NoError(t, err)
			if tt.want == "" {
				assert.NotContains(t, string(out), "updatedAt")
			} else {
				assert.Contains(t, string(out), tt.want)
			}
		})
	}
}

func TestBuildRejectsInvalidModels(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Model)
		wantErr string
	}{
		{name: "url and data", mutate: func(m *Model) { m.Resources[0].Data = map[string]any{"a": 1} }, wantErr: "exactly one of url or data"},
		{name: "neither url nor data", mutate: func(m *Model) { m.Resources[0].URL = "" }, wantErr: "exactly one of url or data"},
		{name: "relative url", mutate: func(m *Model) { m.Resources[0].URL = "/skills/x" }, wantErr: "https"},
		{name: "http url", mutate: func(m *Model) { m.Resources[0].URL = "http://example.com/x" }, wantErr: "https"},
		{name: "localhost publisher", mutate: func(m *Model) { m.Publisher = "localhost" }, wantErr: "fully qualified"},
		{name: "empty namespace", mutate: func(m *Model) { m.Namespace = "" }, wantErr: "namespace"},
		{name: "bad name", mutate: func(m *Model) { m.Resources[0].Name = "my skill" }, wantErr: "name"},
		{name: "unknown kind", mutate: func(m *Model) { m.Resources[0].Kind = "agent" }, wantErr: "kind"},
		{
			name: "duplicate identifier",
			mutate: func(m *Model) {
				dup := m.Resources[0]
				dup.Kind = KindMCPServer
				m.Resources = append(m.Resources, dup)
			},
			wantErr: "more than one resource",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			m := oneSkill()
			tt.mutate(&m)

			// Act
			out, _, err := Build(m)

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, out)
		})
	}
}

func TestBuildQueryFindings(t *testing.T) {
	tests := []struct {
		name      string
		queries   []string
		wantCount int
		wantRules []string
	}{
		{name: "none", queries: nil, wantCount: 0, wantRules: []string{RuleQueries}},
		{name: "one", queries: []string{"deploy"}, wantCount: 1, wantRules: []string{RuleQueries}},
		{name: "duplicates collapse to one", queries: []string{"deploy it", " Deploy  it "}, wantCount: 1, wantRules: []string{RuleQueries}},
		{name: "two", queries: []string{"a", "b"}, wantCount: 2},
		{name: "six are cut to five", queries: []string{"1", "2", "3", "4", "5", "6"}, wantCount: 5, wantRules: []string{RuleQueries}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			m := oneSkill()
			m.Resources[0].RepresentativeQueries = tt.queries

			// Act
			out, findings, err := Build(m)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantRules, uniqueRules(findings))
			for _, f := range findings {
				assert.Equal(t, SeverityWarning, f.Severity)
				assert.Equal(t, "urn:air:example.com:ai-rulez:deploy", f.Identifier)
			}
			validated, err := Validate(out)
			require.NoError(t, err)
			assert.False(t, HasErrors(validated))
			assert.Equal(t, tt.wantCount, countQueries(t, out))
		})
	}
}

func TestBuildMediaTypes(t *testing.T) {
	tests := []struct {
		name      string
		kind      Kind
		overrides map[Kind]string
		want      string
	}{
		{name: "skill", kind: KindSkill, want: MediaTypeSkill},
		{name: "mcp server", kind: KindMCPServer, want: MediaTypeMCPServer},
		{name: "plugin default", kind: KindPlugin, want: DefaultMediaTypePlugin},
		{name: "plugin override", kind: KindPlugin, overrides: map[Kind]string{KindPlugin: "application/vnd.example.plugin+json"}, want: "application/vnd.example.plugin+json"},
		{name: "skill override", kind: KindSkill, overrides: map[Kind]string{KindSkill: `text/markdown; profile="urn:air:agent-skills"`}, want: `text/markdown; profile=\"urn:air:agent-skills\"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			m := oneSkill()
			m.Resources[0].Kind = tt.kind
			m.MediaTypes = tt.overrides

			// Act
			out, _, err := Build(m)

			// Assert
			require.NoError(t, err)
			assert.Contains(t, string(out), `"type": "`+tt.want+`"`)
		})
	}
}

func TestBuildRejectsAnEmptyMediaTypeOverride(t *testing.T) {
	// Arrange
	m := oneSkill()
	m.MediaTypes = map[Kind]string{KindSkill: " "}

	// Act
	_, _, err := Build(m)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "media type")
}

func TestBuildKeepsAnEmptyDataObject(t *testing.T) {
	// Arrange: an empty but present data object still satisfies url-xor-data.
	m := oneSkill()
	m.Resources[0].URL = ""
	m.Resources[0].Data = map[string]any{}

	// Act
	out, _, err := Build(m)

	// Assert
	require.NoError(t, err)
	assert.Contains(t, string(out), `"data": {}`)
}

func TestBuildDoesNotEscapeHTML(t *testing.T) {
	// Arrange
	m := oneSkill()
	m.Resources[0].Description = "Deploy <service> & verify"

	// Act
	out, _, err := Build(m)

	// Assert
	require.NoError(t, err)
	assert.Contains(t, string(out), `"description": "Deploy <service> & verify"`)
}

func TestBuildNeverEmitsTrustManifest(t *testing.T) {
	// Act
	out, _, err := Build(fixtureModel())

	// Assert
	require.NoError(t, err)
	assert.NotContains(t, string(out), "rustManifest")
}

func oneSkill() Model {
	return Model{
		Publisher: "example.com",
		Namespace: "ai-rulez",
		Resources: []Resource{{
			Kind: KindSkill, Name: "deploy", URL: "https://example.com/skills/deploy/SKILL.md",
			RepresentativeQueries: []string{"deploy the app", "ship to production"},
		}},
	}
}

func countQueries(t *testing.T, out []byte) int {
	t.Helper()
	var doc struct {
		Entries []struct {
			Queries []string `json:"representativeQueries"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal(out, &doc))
	require.Len(t, doc.Entries, 1)
	return len(doc.Entries[0].Queries)
}
