package lint

import (
	"context"
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
