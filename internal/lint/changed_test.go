package lint

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lintReport(t *testing.T, base string) *Report {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), base)
	require.NoError(t, err)
	tree, err := LoadTree(base)
	require.NoError(t, err)
	rep, err := Run(cfg, tree)
	require.NoError(t, err)
	return rep
}

func changedProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":                 baseConfig,
		".ai-rulez/rules/links.md":              "---\ndescription: links to the guide\n---\nSee [guide](../context/guide.md) and [gone](missing.md).\n",
		".ai-rulez/rules/names.md":              "---\ndescription: names the helper skill\n---\nUse the `code-helper` skill when stuck, and the `ghost-writer` skill too.\n",
		".ai-rulez/rules/unrelated.md":          "---\ndescription: untouched\n---\nSee [nothing](nowhere.md).\n",
		".ai-rulez/context/guide.md":            "---\ndescription: the guide\n---\n# Guide\n",
		".ai-rulez/skills/code-helper/SKILL.md": "---\nname: code-helper\ndescription: Use when you need the helper skill to do things.\n---\n# Code helper\n",
		".ai-rulez/skills/other/SKILL.md":       "---\nname: other\ndescription: Use when you need some other skill to do things.\n---\nSee [x](gone-too.md).\n",
	})
	gitAdd(t, root)
	return root
}

func TestDepsRecordLinksAndNames(t *testing.T) {
	rep := lintReport(t, changedProject(t))
	assert.Contains(t, rep.Deps[".ai-rulez/rules/links.md"], ".ai-rulez/context/guide.md", "a resolved link")
	assert.Contains(t, rep.Deps[".ai-rulez/rules/links.md"], ".ai-rulez/rules/missing.md", "an unresolved target is remembered too")
	assert.Contains(t, rep.Deps[".ai-rulez/rules/names.md"], ".ai-rulez/skills/code-helper/SKILL.md", "a skill named in prose")
}

func TestNarrowToChangedKeepsChangedAndDependents(t *testing.T) {
	rep := lintReport(t, changedProject(t))
	all := len(rep.Findings)
	require.Greater(t, all, 3)

	// The guide changed: links.md refers to it, so its findings stay; unrelated.md does not.
	scope := NarrowToChanged(rep, []string{".ai-rulez/context/guide.md"}, "main")

	files := map[string]bool{}
	for i := range rep.Findings {
		files[rep.Findings[i].RepoPath()] = true
	}
	assert.True(t, files[".ai-rulez/rules/links.md"])
	assert.False(t, files[".ai-rulez/rules/unrelated.md"])
	assert.False(t, files[".ai-rulez/skills/other/SKILL.md"])
	assert.Equal(t, 1, scope.Changed)
	assert.Equal(t, 1, scope.Dependents)
	assert.Equal(t, all-len(rep.Findings), scope.Dropped)
}

func TestNarrowToChangedKeepsFindingsOfChangedFile(t *testing.T) {
	rep := lintReport(t, changedProject(t))
	NarrowToChanged(rep, []string{".ai-rulez/rules/unrelated.md"}, "HEAD")
	require.NotEmpty(t, rep.Findings)
	for i := range rep.Findings {
		assert.Equal(t, ".ai-rulez/rules/unrelated.md", rep.Findings[i].RepoPath())
	}
}

func TestNarrowToChangedFollowsNameReferences(t *testing.T) {
	rep := lintReport(t, changedProject(t))
	// helper's SKILL.md changed: names.md mentions it (and has its own AR301 for ghost).
	NarrowToChanged(rep, []string{".ai-rulez/skills/code-helper/SKILL.md"}, "HEAD")
	assert.True(t, has(rep.Findings, CodeReferenceUnknown, "names.md", 0))
}

// chainProject has a reference chain top -> mid -> leaf and a cycle a <-> b, and
// every file carries one broken link so it has a finding of its own.
func chainProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	rule := func(desc, body string) string {
		return "---\ndescription: " + desc + "\n---\n" + body + " [gone](gone.md)\n"
	}
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":        baseConfig,
		".ai-rulez/context/leaf.md":    rule("the leaf", "# Leaf"),
		".ai-rulez/context/mid.md":     rule("refers to the leaf", "See [leaf](leaf.md)."),
		".ai-rulez/rules/top.md":       rule("refers to mid", "See [mid](../context/mid.md)."),
		".ai-rulez/rules/a.md":         rule("cycle a", "See [b](b.md)."),
		".ai-rulez/rules/b.md":         rule("cycle b", "See [a](a.md)."),
		".ai-rulez/rules/unrelated.md": rule("unrelated", "Nothing."),
	})
	gitAdd(t, root)
	return root
}

func narrowedFiles(rep *Report) map[string]string {
	out := map[string]string{}
	for i := range rep.Findings {
		if rep.Findings[i].Code == CodeLinkUnresolved {
			out[rep.Findings[i].RepoPath()] = rep.Findings[i].Hop()
		}
	}
	return out
}

func TestNarrowToChangedDepth(t *testing.T) {
	const leaf = ".ai-rulez/context/leaf.md"
	tests := []struct {
		name    string
		changed []string
		depth   int
		want    map[string]string
	}{
		{"depth 1 is one hop (the default)", []string{leaf}, 1, map[string]string{
			leaf: "changed", ".ai-rulez/context/mid.md": "dependent"}},
		{"depth 2 reaches a file that refers to a dependent", []string{leaf}, 2, map[string]string{
			leaf: "changed", ".ai-rulez/context/mid.md": "dependent", ".ai-rulez/rules/top.md": "transitive(2)"}},
		{"all follows the whole chain", []string{leaf}, DepthAll, map[string]string{
			leaf: "changed", ".ai-rulez/context/mid.md": "dependent", ".ai-rulez/rules/top.md": "transitive(2)"}},
		{"a cycle terminates", []string{".ai-rulez/rules/a.md"}, DepthAll, map[string]string{
			".ai-rulez/rules/a.md": "changed", ".ai-rulez/rules/b.md": "dependent"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			rep := lintReport(t, chainProject(t))

			// Act
			scope := NarrowToChangedWith(rep, tt.changed, "main", NarrowOptions{Depth: tt.depth})

			// Assert
			assert.Equal(t, tt.want, narrowedFiles(rep))
			assert.Equal(t, tt.depth, scope.Depth)
			assert.Equal(t, len(tt.changed), scope.Changed)
		})
	}
}

func TestNarrowToChangedClosureIsDeterministicAndCapped(t *testing.T) {
	changed := []string{".ai-rulez/context/leaf.md"}
	root := chainProject(t)
	first := lintReport(t, root)
	second := lintReport(t, root)
	a := NarrowToChangedWith(first, changed, "main", NarrowOptions{Depth: DepthAll})
	b := NarrowToChangedWith(second, changed, "main", NarrowOptions{Depth: DepthAll})
	assert.Equal(t, findingKeys(first.Findings), findingKeys(second.Findings))
	assert.Equal(t, a, b)
	assert.Equal(t, 1, a.Dependents)
	assert.Equal(t, 1, a.Transitive)

	capped := lintReport(t, root)
	scope := NarrowToChangedWith(capped, changed, "main", NarrowOptions{Depth: DepthAll, MaxFiles: 1})
	assert.Equal(t, map[string]string{".ai-rulez/context/leaf.md": "changed", ".ai-rulez/context/mid.md": "dependent"}, narrowedFiles(capped),
		"the cap keeps the nearest files first")
	assert.Equal(t, 1, scope.Truncated)
}

func TestNarrowToChangedHopIsInJSONOnlyWhenNarrowed(t *testing.T) {
	rep := lintReport(t, chainProject(t))
	plain, err := json.Marshal(rep.Findings[0])
	require.NoError(t, err)
	assert.NotContains(t, string(plain), `"hop"`)

	NarrowToChangedWith(rep, []string{".ai-rulez/context/leaf.md"}, "main", NarrowOptions{Depth: 2})
	out, err := json.Marshal(rep.Findings)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"hop":"transitive(2)"`)
}
