package mcp

import (
	"context"
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// monorepoProject is a repository with a scope at three places: the root, an api
// package that overrides the root's lint skill, and a web package.
func monorepoProject(t *testing.T) string {
	t.Helper()
	root := project(t, discoverConfig, map[string]string{"skills/own/SKILL.md": repoSkill("own")})
	for rel, content := range map[string]string{
		".claude/skills/lint/SKILL.md":            "---\nname: lint\ndescription: Lint the root with the root linter\n---\n\n# lint\n",
		"pkg/api/.claude/skills/lint/SKILL.md":    "---\nname: lint\ndescription: Lint the api package with the api linter\n---\n\n# lint api\n",
		"pkg/api/.claude/skills/handler/SKILL.md": "---\nname: handler\ndescription: Write an api handler\n---\n\n# handler\n",
		"pkg/web/skills/ui/SKILL.md":              "---\nname: ui\ndescription: Build a web ui component\n---\n\n# ui\n",
		"AGENTS.md":                               "# Root conventions\n\nBe terse.\n",
		"pkg/api/AGENTS.md":                       "# API conventions\n\nWrap every error.\n",
		".claude/rules/style.md":                  "---\ndescription: Code style for the whole repository\n---\n\nUse tabs.\n",
		"pkg/api/.claude/agents/reviewer.md":      "---\ndescription: Reviews changes to the api\n---\n\nReview carefully.\n",
	} {
		writeFile(t, root, rel, content)
	}
	return root
}

func resultNames(t *testing.T, out map[string]any) []string {
	t.Helper()
	var names []string
	results, _ := out["results"].([]any)
	for _, r := range results {
		names = append(names, r.(map[string]any)["name"].(string))
	}
	return names
}

func TestFindSkill_PathAndScope(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want []string
	}{
		{
			name: "a path inside a package gets the package's skills, the nearer lint shadowing the root's",
			args: map[string]any{"task": "lint the code", "path": "pkg/api/handler.go"},
			want: []string{"pkg.api.claude.skills.lint"},
		},
		{
			name: "a path elsewhere gets the root's lint",
			args: map[string]any{"task": "lint the code", "path": "pkg/web/index.ts"},
			want: []string{"lint"},
		},
		{
			name: "a package skill is out of reach from a sibling package",
			args: map[string]any{"task": "write an api handler", "path": "pkg/web/index.ts"},
			want: nil,
		},
		{
			name: "a skill ai-rulez defines applies everywhere",
			args: map[string]any{"task": "the own skill that lives in the repository", "path": "pkg/web/index.ts"},
			want: []string{"own"},
		},
		{
			name: "an exact scope",
			args: map[string]any{"task": "lint api handler", "scope": "pkg/api"},
			want: []string{"pkg.api.claude.skills.lint", "handler"},
		},
		{
			name: "the root scope is not recursive by default",
			args: map[string]any{"task": "lint api handler web ui", "scope": "."},
			want: []string{"lint"},
		},
		{
			name: "recursive adds the descendants",
			args: map[string]any{"task": "lint handler ui", "scope": ".", "recursive": true},
			want: []string{"lint", "pkg.api.claude.skills.lint", "handler", "ui"},
		},
		{
			name: "recursive below the root",
			args: map[string]any{"task": "lint handler ui", "scope": "pkg/api", "recursive": true},
			want: []string{"pkg.api.claude.skills.lint", "handler"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			srv := newServerFor(t, &ServeSetup{WorkDir: monorepoProject(t), DiscoverSkills: "repo"})
			session := connect(t, srv)

			// Act
			out, isErr := toolJSON(t, session, "find_skill", tt.args)

			// Assert
			require.False(t, isErr, "%v", out)
			assert.ElementsMatch(t, tt.want, resultNames(t, out))
		})
	}
}

func TestFindSkill_PathReportsTheResolvedScopesAndEachResultsScope(t *testing.T) {
	// Arrange
	srv := newServerFor(t, &ServeSetup{WorkDir: monorepoProject(t), DiscoverSkills: "repo"})
	session := connect(t, srv)

	// Act
	out, isErr := toolJSON(t, session, "find_skill", map[string]any{"task": "write an api handler", "path": "pkg/api/x/y.go"})

	// Assert
	require.False(t, isErr)
	assert.Equal(t, []any{"pkg/api", "."}, out["scopes"])
	assert.Equal(t, "pkg/api/x/y.go", out["path"])
	first := out["results"].([]any)[0].(map[string]any)
	assert.Equal(t, "handler", first["name"])
	assert.Equal(t, "pkg/api", first["scope"])
}

func TestFindSkill_RejectsBadScopeArguments(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"path and scope together", map[string]any{"task": "x", "path": "a", "scope": "b"}, "either path or scope"},
		{"recursive without a scope", map[string]any{"task": "x", "path": "a", "recursive": true}, "recursive"},
		{"parent escape", map[string]any{"task": "x", "path": "../outside"}, "leaves the project"},
		{"buried parent escape", map[string]any{"task": "x", "path": "pkg/../../outside"}, "leaves the project"},
		{"absolute path", map[string]any{"task": "x", "path": "/etc/passwd"}, "relative"},
		{"absolute scope", map[string]any{"task": "x", "scope": "/etc"}, "relative"},
		{"backslash", map[string]any{"task": "x", "path": "pkg\\api"}, "backslash"},
		{"unknown scope", map[string]any{"task": "x", "scope": "pkg/none"}, "no scope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServerFor(t, &ServeSetup{WorkDir: monorepoProject(t), DiscoverSkills: "repo"})
			session := connect(t, srv)

			res := call(t, session, "find_skill", tt.args)

			require.True(t, res.IsError)
			assert.Contains(t, firstText(res), tt.want)
		})
	}
}

func TestFindSkill_PathNeedsDiscovery(t *testing.T) {
	srv := newServerFor(t, &ServeSetup{WorkDir: monorepoProject(t)})
	session := connect(t, srv)

	res := call(t, session, "find_skill", map[string]any{"task": "lint", "path": "pkg/api"})

	require.True(t, res.IsError)
	assert.Contains(t, firstText(res), "discover")
}

func TestFindSkill_WithoutPathOrScopeIsUnchanged(t *testing.T) {
	srv := newServerFor(t, &ServeSetup{WorkDir: monorepoProject(t), DiscoverSkills: "repo"})
	session := connect(t, srv)

	out, isErr := toolJSON(t, session, "find_skill", map[string]any{"task": "lint the code"})

	require.False(t, isErr)
	assert.ElementsMatch(t, []string{"lint", "pkg.api.claude.skills.lint"}, resultNames(t, out))
	assert.NotContains(t, out, "scopes")
	assert.NotContains(t, out, "path")
}

// catalogDoc reads the ai-rulez://catalog resource of a merged server.
func catalogDoc(t *testing.T, root, uri string) map[string]any {
	t.Helper()
	srv := newServerFor(t, &ServeSetup{WorkDir: root, DiscoverSkills: "repo", Authoring: []Option{WithRoot(root)}})
	session := connect(t, srv)
	read, err := session.ReadResource(context.Background(), &sdkmcp.ReadResourceParams{URI: uri})
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(read.Contents[0].Text), &doc))
	return doc
}

func scopeEntries(t *testing.T, doc map[string]any) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	list, ok := doc["scopes"].([]any)
	require.True(t, ok, "the catalog has a scopes section: %v", doc)
	for _, e := range list {
		entry := e.(map[string]any)
		out[entry["scope"].(string)] = entry
	}
	return out
}

func TestCatalogResource_ScopesSection(t *testing.T) {
	// Arrange
	root := monorepoProject(t)

	// Act
	doc := catalogDoc(t, root, "ai-rulez://catalog")

	// Assert
	scopes := scopeEntries(t, doc)
	assert.Len(t, scopes, 3)
	assert.Equal(t, map[string]any{"skill": float64(1), "instruction": float64(1), "rule": float64(1)}, scopes["."]["counts"])
	assert.Equal(t, map[string]any{"skill": float64(2), "instruction": float64(1), "agent": float64(1)}, scopes["pkg/api"]["counts"])
	assert.Equal(t, []any{"agents", "claude"}, scopes["pkg/api"]["sources"])
	assert.Equal(t, []any{"skills-dir"}, scopes["pkg/web"]["sources"])
	discovery := doc["skill_discovery"].(map[string]any)
	index := discovery["index"].(map[string]any)
	assert.Contains(t, index, "cache", "the index reports how it used the cache")
	delete(index, "cache")
	assert.Equal(t, map[string]any{"items": float64(8), "limit": float64(20000), "truncated": false, "found": float64(8)}, index)
}

func TestCatalogResource_ScopesCanBeNarrowed(t *testing.T) {
	root := monorepoProject(t)
	tests := []struct {
		name string
		uri  string
		want []string
	}{
		{"by path", "ai-rulez://catalog?path=pkg/api/handler.go", []string{".", "pkg/api"}},
		{"by exact scope", "ai-rulez://catalog?scope=pkg/web", []string{"pkg/web"}},
		{"by scope with descendants", "ai-rulez://catalog?scope=.&recursive=true", []string{".", "pkg/api", "pkg/web"}},
		{"a scope is not recursive unless asked", "ai-rulez://catalog?scope=.", []string{"."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := catalogDoc(t, root, tt.uri)

			var got []string
			for scope := range scopeEntries(t, doc) {
				got = append(got, scope)
			}
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

func TestCatalogResource_BadScopeQueryIsRefused(t *testing.T) {
	root := monorepoProject(t)
	srv := newServerFor(t, &ServeSetup{WorkDir: root, DiscoverSkills: "repo", Authoring: []Option{WithRoot(root)}})
	session := connect(t, srv)

	for _, uri := range []string{"ai-rulez://catalog?path=../x", "ai-rulez://catalog?scope=nope", "ai-rulez://catalog?bogus=1", "ai-rulez://catalog?path=a&scope=b"} {
		t.Run(uri, func(t *testing.T) {
			_, err := session.ReadResource(context.Background(), &sdkmcp.ReadResourceParams{URI: uri})

			require.Error(t, err)
		})
	}
}

func TestCatalogResource_WithoutDiscoveryHasNoScopes(t *testing.T) {
	root := monorepoProject(t)
	srv := newServerFor(t, &ServeSetup{WorkDir: root, Authoring: []Option{WithRoot(root)}})
	session := connect(t, srv)

	read, err := session.ReadResource(context.Background(), &sdkmcp.ReadResourceParams{URI: "ai-rulez://catalog"})

	require.NoError(t, err)
	assert.NotContains(t, read.Contents[0].Text, `"scopes"`)
}

func TestDiscovery_IndexOverItsLimitWarnsAtStartupAndKeepsTheShallowScopes(t *testing.T) {
	// Arrange
	root := monorepoProject(t)
	log := &testutil.LogRecorder{}

	// Act
	srv := newServerFor(t, &ServeSetup{WorkDir: root, DiscoverSkills: "repo", Log: log, indexLimit: 4})

	// Assert
	assert.Contains(t, log.String(), "WARN The repository index is over its limit")
	report := srv.Catalog().Discovery().Index
	require.NotNil(t, report)
	report.Cache = nil
	assert.Equal(t, IndexReport{Items: 4, Limit: 4, Truncated: true, Found: 8}, *report)
	_, session := srv, connect(t, srv)
	out, isErr := toolJSON(t, session, "find_context", map[string]any{"scope": ".", "recursive": true})
	require.False(t, isErr)
	assert.Equal(t, []string{"instruction:.:AGENTS.md", "rule:.:style", "agent:pkg/api:reviewer"}, contextKeys(t, out),
		"the root scope is kept whole; the cut falls inside the first deeper scope")
}

func TestDiscovery_IndexUnderItsLimitDoesNotWarn(t *testing.T) {
	log := &testutil.LogRecorder{}

	newServerFor(t, &ServeSetup{WorkDir: monorepoProject(t), DiscoverSkills: "repo", Log: log})

	assert.NotContains(t, log.String(), "over its limit")
}
