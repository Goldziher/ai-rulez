package generator

import (
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/plugin"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyKindName(t *testing.T) {
	tests := map[string]struct{ kind, name string }{
		"/b/skills/deploy/SKILL.md":                  {"skill", "deploy"},
		"/b/.cursor-plugin/skills/deploy/SKILL.md":   {"skill", "deploy"},
		"/b/commands/extract.md":                     {"command", "extract"},
		"/b/agents/reviewer.md":                      {"agent", "reviewer"},
		"/b/.github/plugin/agents/reviewer.agent.md": {"agent", "reviewer"},
		"/b/skills/deploy/references/cli.md":         {"", ""},
		"/b/commands/extract.toml":                   {"", ""},
		"/b/.claude-plugin/plugin.json":              {"", ""},
	}
	for path, want := range tests {
		kind, name := copyKindName(path)
		assert.Equal(t, want.kind, kind, path)
		assert.Equal(t, want.name, name, path)
	}
}

func TestUniqueByNameDropsSharedNames(t *testing.T) {
	got := uniqueByName([]config.ContentFile{{Name: "a", Path: "/1"}, {Name: "b", Path: "/2"}, {Name: "a", Path: "/3"}, {Name: "a", Path: "/4"}})
	assert.Len(t, got, 1)
	assert.Equal(t, "/2", got["b"].Path)
}

func TestNewCopyBundleResolvesLinks(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	testutil.SymlinkOrSkip(t, real, link)
	bundle := newCopyBundle(link, &plugin.Manifest{})
	resolved, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)
	assert.Equal(t, resolved, bundle.dir)
}
