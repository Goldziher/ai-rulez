package mcp

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/repoindex"
)

// excludeProject is the monorepo plus a vendored tree that keeps skills, an
// instruction file, a rule and an agent of its own.
func excludeProject(t *testing.T, mcpTable string) string {
	t.Helper()
	root := monorepoProject(t)
	writeFile(t, root, ".ai-rulez/config.toml", discoverConfig+mcpTable)
	writeFile(t, root, "third_party/AGENTS.md", "# Vendored conventions\n")
	writeFile(t, root, "third_party/.claude/skills/dep/SKILL.md", repoSkill("dep"))
	writeFile(t, root, "third_party/.claude/rules/vendored.md", "---\ndescription: A vendored rule\n---\n\nbody\n")
	writeFile(t, root, "third_party/.claude/agents/vendor-agent.md", "---\ndescription: A vendored agent\n---\n\nbody\n")
	return root
}

func TestDiscoverSkills_ExcludeLeavesPathsOutOfSkillsAndContext(t *testing.T) {
	tests := []struct {
		name      string
		mcpTable  string
		cli       []string
		wantSkill bool
		wantScope bool
	}{
		{name: "nothing excluded", wantSkill: true, wantScope: true},
		{name: "the configuration excludes", mcpTable: "[mcp]\nexclude = [\"third_party/**\"]\n"},
		{name: "the flag excludes", cli: []string{"third_party/**"}},
		{name: "a glob without a slash matches at any depth", cli: []string{"third_party"}},
		{
			name: "the flag adds to the configuration", mcpTable: "[mcp]\nexclude = [\"pkg/web/**\"]\n", cli: []string{"third_party/**"},
		},
		{name: "a glob that matches another path leaves it served", cli: []string{"other/**"}, wantSkill: true, wantScope: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := excludeProject(t, tt.mcpTable)
			srv, session := contextServer(t, root, func(s *ServeSetup) { s.Exclude = tt.cli })

			// Act
			out, isErr := toolJSON(t, session, "find_context", map[string]any{"scope": "third_party"})

			// Assert
			_, skill := srv.Catalog().Lookup("dep")
			assert.Equal(t, tt.wantSkill, skill)
			if tt.wantScope {
				require.False(t, isErr, "%v", out)
				assert.Equal(t, []string{"agent:third_party:vendor-agent", "instruction:third_party:AGENTS.md", "rule:third_party:vendored"}, contextKeys(t, out))
			} else {
				assert.True(t, isErr, "the scope is not indexed: %v", out)
			}
			_, plain := srv.Catalog().Lookup("lint")
			assert.True(t, plain, "what is not excluded is still served")
		})
	}
}

func TestDiscoverSkills_ExcludeBothSourcesAreCombined(t *testing.T) {
	// Arrange
	root := excludeProject(t, "[mcp]\nexclude = [\"pkg/web/**\"]\n")

	// Act
	srv, _ := contextServer(t, root, func(s *ServeSetup) { s.Exclude = []string{"third_party/**"} })

	// Assert
	_, web := srv.Catalog().Lookup("ui")
	_, dep := srv.Catalog().Lookup("dep")
	assert.False(t, web, "the configuration's glob applies")
	assert.False(t, dep, "the flag's glob applies")
}

func TestDiscoverSkills_ExcludeNeedsDiscovery(t *testing.T) {
	// Arrange
	root := excludeProject(t, "")
	setup := &ServeSetup{WorkDir: root, Exclude: []string{"third_party/**"}}

	// Act
	_, err := setup.NewServer(t.Context())

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--exclude")
	assert.Contains(t, err.Error(), "discover")
}

func TestDiscoverSkills_ExcludeRejectsABadGlob(t *testing.T) {
	root := excludeProject(t, "")
	setup := &ServeSetup{WorkDir: root, DiscoverSkills: "repo", Exclude: []string{"../outside/**"}}

	_, err := setup.NewServer(t.Context())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "exclude")
}

func TestIndexWatchIgnore_Exclude(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	ex, err := repoindex.NewExcluder([]string{"gen/**", "**/*.generated.md"})
	require.NoError(t, err)
	ignore := indexWatchIgnore(root, nil, ex)

	tests := []struct {
		rel  string
		want bool
	}{
		{rel: "gen", want: true},
		{rel: "gen/deep/AGENTS.md", want: true},
		{rel: "pkg/x.generated.md", want: true},
		{rel: "pkg/AGENTS.md"},
		{rel: "generated"},
	}
	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			assert.Equal(t, tt.want, ignore(filepath.Join(root, filepath.FromSlash(tt.rel))))
		})
	}
}

func TestIndexWatch_ExcludedPathsDoNotRefreshTheIndex(t *testing.T) {
	// Arrange
	root := monorepoProject(t)
	writeFile(t, root, "src/other.go", "package main\n")
	var rebuilds atomic.Int32
	_, session, _ := watchedServer(t, root, func(s *ServeSetup) {
		s.Exclude = []string{"gen/**"}
		s.afterLoad = func() { rebuilds.Add(1) }
	})
	base := settled(t, &rebuilds)

	// Act
	writeFile(t, root, "gen/AGENTS.md", "# Generated tree\n")
	time.Sleep(600 * time.Millisecond)

	// Assert
	assert.Equal(t, base, rebuilds.Load(), "a change below an excluded path is no index change")
	assert.Zero(t, contextCount(t, session, "gen"))
}
