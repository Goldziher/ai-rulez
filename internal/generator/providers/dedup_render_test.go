package providers_test

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// claudeMD renders the claude preset for the given content/config and returns
// the CLAUDE.md body.
func claudeMD(t *testing.T, content *config.ContentTree, cfg *config.Config) string {
	t.Helper()
	gen, err := providers.LoadBuiltin("claude")
	require.NoError(t, err)
	outputs, err := gen.Generate(content, "/test", cfg)
	require.NoError(t, err)
	for _, o := range outputs {
		if !o.IsDir && strings.HasSuffix(o.Path, "CLAUDE.md") {
			return o.Content
		}
	}
	t.Fatal("CLAUDE.md not emitted")
	return ""
}

// collidingTree models the xberg-io scenario: a root/include rule and a builtin
// domain rule that share the name "commit-messages".
func collidingTree() *config.ContentTree {
	return &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "commit-messages", Content: "LOCAL WORDING", Metadata: &config.Metadata{Priority: "high"}},
		},
		Domains: map[string]*config.Domain{
			"git-workflow": {
				Name:    "git-workflow",
				Builtin: true,
				Rules: []config.ContentFile{
					{Name: "commit-messages", Content: "BUILTIN WORDING", Metadata: &config.Metadata{Priority: "high"}},
					{Name: "atomic-commits", Content: "atomic", Metadata: &config.Metadata{Priority: "high"}},
				},
			},
		},
	}
}

func TestClaude_DeduplicatesCollidingRuleName(t *testing.T) {
	t.Parallel()

	body := claudeMD(t, collidingTree(), &config.Config{Name: "test"})

	assert.Equal(t, 1, strings.Count(body, "### commit-messages\n"), "collided rule must render exactly once")
	assert.Contains(t, body, "LOCAL WORDING", "root content wins over the builtin")
	assert.NotContains(t, body, "BUILTIN WORDING", "builtin copy must be dropped")
	// Two unique rules survive: commit-messages (deduped) + atomic-commits.
	assert.Contains(t, body, "Content: rules=2", "header count reflects the deduplicated set")
}

func TestClaude_CompactOmitsPriority(t *testing.T) {
	t.Parallel()

	compact := true
	body := claudeMD(t, collidingTree(), &config.Config{Name: "test", Compact: &compact})

	assert.NotContains(t, body, "**Priority:**", "compact mode omits per-rule priority annotations")
	assert.Contains(t, body, "### commit-messages", "rules are still rendered in compact mode")
}

func TestClaude_NonCompactKeepsPriority(t *testing.T) {
	t.Parallel()

	body := claudeMD(t, collidingTree(), &config.Config{Name: "test"})
	assert.Contains(t, body, "**Priority:** high", "default rendering keeps priority annotations")
}
