package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch/setup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// conceptEmbedder maps words to concept dimensions, so synonyms embed close
// together without a model.
type conceptEmbedder struct {
	groups [][]string
	err    error
	calls  int
	model  string
}

func (c *conceptEmbedder) Embed(_ context.Context, texts []string) (skillsearch.Embedding, error) {
	c.calls++
	if c.err != nil {
		return skillsearch.Embedding{}, c.err
	}
	out := skillsearch.Embedding{CostKnown: true}
	for _, text := range texts {
		v := make([]float32, len(c.groups)+1)
		for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return r < 'a' || r > 'z' }) {
			hit := false
			for g, words := range c.groups {
				for _, x := range words {
					if x == w {
						v[g]++
						hit = true
					}
				}
			}
			if !hit {
				v[len(c.groups)] += 0.05
			}
		}
		out.Vectors = append(out.Vectors, v)
	}
	return out, nil
}
func (c *conceptEmbedder) Fingerprint() string { return "test@local" }
func (c *conceptEmbedder) Model() string       { return c.model }

func searchEmbedder() *conceptEmbedder {
	return &conceptEmbedder{model: "concepts", groups: [][]string{
		{"refund", "money", "back", "customer", "returns", "reimburse"},
		{"deploy", "staging", "cluster", "service"},
		{"git", "branching", "commit"},
	}}
}

func searchCatalog(t *testing.T) *Catalog {
	t.Helper()
	served := []generator.ServedSkill{
		servedSkill("issue-refund", "", "Reimburse a customer for returns", nil),
		servedSkill("deploy-staging", "", "Deploy a service to the staging cluster", nil),
		servedSkill("git-workflow", "", "Branching and commit conventions", nil),
	}
	cat, err := BuildCatalog("p", "claude", served, SkillFilter{})
	require.NoError(t, err)
	return cat
}

// searchServer starts a skills server whose find_skill ranks with mode, over an
// index built from the catalog (when build is true).
func searchServer(t *testing.T, mode string, build bool, emb *conceptEmbedder) (*rpcPeer, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cat := searchCatalog(t)
	dir := filepath.Join(t.TempDir(), ".ai-rulez")
	cfg := &config.Config{ConfigDir: dir, Search: &skillsearch.Config{Mode: mode}}
	if build {
		res, err := skillsearch.Build(t.Context(), catalogItems(cat.Skills()), &skillsearch.BuildOptions{Config: *cfg.Search, Embedder: emb})
		require.NoError(t, err)
		require.NoError(t, skillsearch.WriteIndex(filepath.Join(dir, "local", "search"), res.Index))
	}
	rt := NewSearchRuntime(func() *config.Config { return cfg })
	rt.newEmbedder = func(*setup.Resolved) (skillsearch.Embedder, func(), error) { return emb, func() {}, nil }
	p, _ := startSkillServerWith(t, cat, ServeOptions{Search: rt})
	return p, dir
}

func firstResult(out map[string]any) string {
	results := out["results"].([]any)
	if len(results) == 0 {
		return ""
	}
	return results[0].(map[string]any)["name"].(string)
}

func TestFindSkill_HybridFindsAParaphrase(t *testing.T) {
	// Arrange: "money back" shares no word with the refund skill's text
	p, _ := searchServer(t, skillsearch.ModeHybrid, true, searchEmbedder())

	// Act
	out, isErr, _ := callTool(t, p, "find_skill", map[string]any{"task": "money back"})

	// Assert
	require.False(t, isErr)
	assert.Equal(t, "issue-refund", firstResult(out))
	assert.Equal(t, "vector", out["ranking"], "hybrid with the default fusion ranks by cosine while every skill has a current vector")
	assert.NotContains(t, out, "degraded")
	first := out["results"].([]any)[0].(map[string]any)
	assert.EqualValues(t, 1, first["vector_rank"])
	assert.EqualValues(t, 0, first["lexical_rank"])
}

func TestFindSkill_LexicalModeIsUnchanged(t *testing.T) {
	emb := searchEmbedder()
	p, _ := searchServer(t, skillsearch.ModeLexical, true, emb)
	emb.calls = 0

	out, isErr, _ := callTool(t, p, "find_skill", map[string]any{"task": "staging cluster"})

	require.False(t, isErr)
	assert.Equal(t, "deploy-staging", firstResult(out))
	assert.NotContains(t, out, "ranking", "the response of a lexical server is unchanged")
	assert.NotContains(t, out, "degraded")
	assert.Zero(t, emb.calls, "lexical mode never embeds")
	first := out["results"].([]any)[0].(map[string]any)
	assert.NotContains(t, first, "lexical_rank")
}

func TestFindSkill_DegradesToLexicalAndSaysWhy(t *testing.T) {
	tests := []struct {
		name  string
		build bool
		err   error
		want  string
	}{
		{"no index", false, nil, "no_index"},
		{"provider down", true, assert.AnError, "provider_unavailable"},
		{"network disabled", true, llm.ErrNetworkDisabled, "network_disabled"},
		{"budget", true, llm.ErrBudget, "budget"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			emb := searchEmbedder()
			p, _ := searchServer(t, skillsearch.ModeHybrid, true, emb)
			if !tt.build {
				p, _ = searchServer(t, skillsearch.ModeHybrid, false, emb)
			}
			emb.err = tt.err

			// Act
			out, isErr, _ := callTool(t, p, "find_skill", map[string]any{"task": "staging cluster"})

			// Assert: still answered, lexically
			require.False(t, isErr)
			assert.Equal(t, tt.want, out["degraded"])
			assert.Equal(t, "lexical", out["ranking"])
			assert.Equal(t, "deploy-staging", firstResult(out))
		})
	}
}

func TestFindSkill_IndexOfAnotherModelIsNotUsed(t *testing.T) {
	// Arrange: index built with model "old", the server embeds with "new"
	old := searchEmbedder()
	old.model = "old"
	p, _ := searchServer(t, skillsearch.ModeHybrid, true, old)
	old.model = "new"

	out, _, _ := callTool(t, p, "find_skill", map[string]any{"task": "staging"})

	assert.Equal(t, "no_index", out["degraded"], "vectors of another model are never mixed in")
}

func TestFindSkill_QueryLogRecordsQueryAndLoad(t *testing.T) {
	// Arrange: only user scope (here the environment) turns the log on
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(setup.LogQueriesEnv, "1")
	emb := searchEmbedder()
	p, dir := searchServer(t, skillsearch.ModeLexical, true, emb)
	_ = p
	cfg := &config.Config{ConfigDir: dir, Search: &skillsearch.Config{Mode: skillsearch.ModeLexical, LogQueries: true}}
	rt := NewSearchRuntime(func() *config.Config { return cfg })
	p, _ = startSkillServerWith(t, searchCatalog(t), ServeOptions{Search: rt})

	// Act
	_, _, _ = callTool(t, p, "find_skill", map[string]any{"task": "staging cluster"})
	_, _, _ = callTool(t, p, "load_skill", map[string]any{"name": "deploy-staging"})

	// Assert
	entries, err := skillsearch.ReadLog(skillsearch.LogPath(dir))
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "query", entries[0].Event)
	assert.Equal(t, "staging cluster", entries[0].Query)
	assert.Equal(t, []string{"deploy-staging"}, entries[0].Results)
	assert.Equal(t, "loaded", entries[1].Event)
	assert.Equal(t, "deploy-staging", entries[1].Skill)
}

func TestFindSkill_RepositoryCannotEnableTheQueryLog(t *testing.T) {
	// Arrange
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(setup.LogQueriesEnv, "")
	_, dir := searchServer(t, skillsearch.ModeLexical, true, searchEmbedder())
	cfg := &config.Config{ConfigDir: dir, Search: &skillsearch.Config{Mode: skillsearch.ModeLexical, LogQueries: true}}
	p, _ := startSkillServerWith(t, searchCatalog(t), ServeOptions{Search: NewSearchRuntime(func() *config.Config { return cfg })})

	// Act
	_, _, _ = callTool(t, p, "find_skill", map[string]any{"task": "staging cluster"})

	// Assert
	entries, err := skillsearch.ReadLog(skillsearch.LogPath(dir))
	require.NoError(t, err)
	assert.Empty(t, entries, "a repository config must not start recording queries")
}

func TestFindSkill_NoQueryLogByDefault(t *testing.T) {
	p, dir := searchServer(t, skillsearch.ModeLexical, true, searchEmbedder())
	_, _, _ = callTool(t, p, "find_skill", map[string]any{"task": "staging cluster"})

	entries, err := skillsearch.ReadLog(skillsearch.LogPath(dir))
	require.NoError(t, err)
	assert.Empty(t, entries)
}
