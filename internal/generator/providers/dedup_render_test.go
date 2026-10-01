package providers_test

import (
	"path/filepath"
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

	// Use the detailed header so the rendered rule count appears in the banner
	// (the default minimal header omits the "Content: rules=N" line).
	cfg := &config.Config{Name: "test", Header: &config.HeaderConfig{Style: "detailed"}}
	body := claudeMD(t, collidingTree(), cfg)

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

// claudeOutputs renders the claude preset and returns the non-directory outputs
// whose path ends with the given suffix.
func claudeOutputs(t *testing.T, content *config.ContentTree, suffix string) []config.OutputFile {
	t.Helper()
	gen, err := providers.LoadBuiltin("claude")
	require.NoError(t, err)
	outputs, err := gen.Generate(content, "/test", &config.Config{Name: "test"})
	require.NoError(t, err)

	want := filepath.ToSlash(suffix)
	var matched []config.OutputFile
	for _, o := range outputs {
		if !o.IsDir && strings.HasSuffix(filepath.ToSlash(o.Path), want) {
			matched = append(matched, o)
		}
	}
	return matched
}

// skillTree models one skill id claimed by the root, an on-disk domain, an
// include-sourced domain and a builtin domain. Fixture domains carry the flags
// directly so the test does not depend on any builtin pack's contents.
func skillTree() *config.ContentTree {
	skill := func(id, source, body string) config.ContentFile {
		return config.ContentFile{Name: id, Path: source + "/skills/" + id + "/SKILL.md", Content: body}
	}
	return &config.ContentTree{
		Skills: []config.ContentFile{skill("shared", "root", "ROOT BODY")},
		Domains: map[string]*config.Domain{
			"ondisk":   {Name: "ondisk", Skills: []config.ContentFile{skill("shared", "ondisk", "ONDISK BODY")}},
			"included": {Name: "included", FromInclude: true, Skills: []config.ContentFile{skill("shared", "include", "INCLUDE BODY")}},
			"testing":  {Name: "testing", Builtin: true, Skills: []config.ContentFile{skill("shared", "builtin", "BUILTIN BODY")}},
		},
	}
}

// TestClaude_CollidingSkillIsWrittenOnce is the end-to-end guard: four sources
// claiming .claude/skills/shared/SKILL.md must produce one output, not four
// writes to one path where the last one silently wins.
func TestClaude_CollidingSkillIsWrittenOnce(t *testing.T) {
	t.Parallel()

	outputs := claudeOutputs(t, skillTree(), "skills/shared/SKILL.md")

	require.Len(t, outputs, 1, "a collided skill id must be emitted exactly once")
	assert.Contains(t, outputs[0].Content, "ROOT BODY", "root skill wins")
	for _, dropped := range []string{"ONDISK BODY", "INCLUDE BODY", "BUILTIN BODY"} {
		assert.NotContains(t, outputs[0].Content, dropped)
	}
}

func TestClaude_CollidingCommandIsWrittenOnce(t *testing.T) {
	t.Parallel()

	command := func(id, source, body string) config.ContentFile {
		return config.ContentFile{Name: id, Path: source + "/commands/" + id + ".md", Content: body}
	}
	tree := &config.ContentTree{
		Commands: []config.ContentFile{command("shared", "root", "ROOT BODY")},
		Domains: map[string]*config.Domain{
			"testing": {Name: "testing", Builtin: true, Commands: []config.ContentFile{command("shared", "builtin", "BUILTIN BODY")}},
		},
	}

	outputs := claudeOutputs(t, tree, "shared/SKILL.md")

	require.Len(t, outputs, 1, "a collided command id must be emitted exactly once")
	assert.Contains(t, outputs[0].Content, "ROOT BODY", "root command wins over the builtin")
	assert.NotContains(t, outputs[0].Content, "BUILTIN BODY")
}

// TestClaude_RendersRulesInPriorityOrder covers #204 end to end: the rendered
// CLAUDE.md lists rules highest-priority first, name breaking ties.
func TestClaude_RendersRulesInPriorityOrder(t *testing.T) {
	t.Parallel()

	content := &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "zeta-low", Content: "low body", Metadata: &config.Metadata{Priority: "low"}},
			{Name: "beta-high", Content: "high body", Metadata: &config.Metadata{Priority: "high"}},
			{Name: "alpha-critical", Content: "critical body", Metadata: &config.Metadata{Priority: "critical"}},
			{Name: "unset", Content: "unset body"},
		},
	}

	body := claudeMD(t, content, &config.Config{Name: "test"})

	alpha := strings.Index(body, "### alpha-critical")
	beta := strings.Index(body, "### beta-high")
	unset := strings.Index(body, "### unset")
	zeta := strings.Index(body, "### zeta-low")
	require.NotEqual(t, -1, alpha)
	require.NotEqual(t, -1, beta)
	require.NotEqual(t, -1, unset)
	require.NotEqual(t, -1, zeta)
	assert.True(t, alpha < beta && beta < unset && unset < zeta,
		"rules render critical → high → medium → low; positions %d %d %d %d", alpha, beta, unset, zeta)
}
