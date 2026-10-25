package repoindex

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func skillMD(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n# " + name + "\n"
}

func ruleMD(desc string) string {
	return "---\ndescription: " + desc + "\n---\n\nBody of the rule.\n"
}

// layout writes files below a fresh directory and returns it.
func layout(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	testutil.WriteTree(t, root, files)
	return root
}

func build(t *testing.T, root string, presets ...string) *Index {
	t.Helper()
	ix, err := Build(context.Background(), Options{Root: root, Presets: presets})
	require.NoError(t, err)
	return ix
}

// keys renders items as "kind:scope:name", sorted.
func keys(items []Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, string(it.Kind)+":"+it.Scope+":"+it.Name)
	}
	sort.Strings(out)
	return out
}

func scopePaths(ix *Index) []string {
	var out []string
	for _, s := range ix.Scopes() {
		out = append(out, s.Path)
	}
	return out
}

// monorepo is a repository with a scope at several depths and conventions.
func monorepo() map[string]string {
	return map[string]string{
		"AGENTS.md":                            "# Root conventions\n\nBe terse.\n",
		"CLAUDE.md":                            "@AGENTS.md\n",
		".claude/skills/lint/SKILL.md":         skillMD("lint", "Lint the root"),
		".claude/rules/style.md":               ruleMD("Style rules"),
		"pkg/api/AGENTS.md":                    "# API conventions\n",
		"pkg/api/.claude/agents/reviewer.md":   ruleMD("Reviews API changes"),
		"pkg/api/.claude/skills/lint/SKILL.md": skillMD("lint", "Lint the API"),
		"pkg/web/.cursor/rules/ui.mdc":         ruleMD("UI rules"),
		"pkg/web/skills/ui/SKILL.md":           skillMD("ui", "Build UI"),
		"docs/readme.md":                       "# Docs\n",
		"node_modules/dep/AGENTS.md":           "# Vendored\n",
	}
}

func TestBuild_InfersScopesAndItems(t *testing.T) {
	tests := []struct {
		name       string
		files      map[string]string
		presets    []string
		wantScopes []string
		wantItems  []string
	}{
		{
			name:       "monorepo with conventions at three depths",
			files:      monorepo(),
			presets:    []string{"claude", "cursor"},
			wantScopes: []string{".", "pkg/api", "pkg/web"},
			wantItems: []string{
				"agent:pkg/api:reviewer",
				"instruction:.:AGENTS.md",
				"instruction:.:CLAUDE.md",
				"instruction:pkg/api:AGENTS.md",
				"rule:.:style",
				"rule:pkg/web:ui",
				"skill:.:lint",
				"skill:pkg/api:lint",
				"skill:pkg/web:ui",
			},
		},
		{
			name:       "a repository with no convention has only the root scope",
			files:      map[string]string{"main.go": "package main\n", "docs/a.md": "# a\n"},
			wantScopes: []string{"."},
		},
		{
			name:       "an empty convention directory still makes a scope",
			files:      map[string]string{"svc/.claude/settings.json": "{}\n"},
			presets:    []string{"claude"},
			wantScopes: []string{".", "svc"},
		},
		{
			name:       "a skills directory without a skill is not a scope",
			files:      map[string]string{"internal/skills/helper.go": "package skills\n"},
			wantScopes: []string{"."},
		},
		{
			name:       "copilot instructions belong to the directory above .github",
			files:      map[string]string{"app/.github/copilot-instructions.md": "# Copilot\n"},
			wantScopes: []string{".", "app"},
			wantItems:  []string{"instruction:app:copilot-instructions.md"},
		},
		{
			name:       "GEMINI.md is an instruction file",
			files:      map[string]string{"tools/GEMINI.md": "# Gemini\n"},
			wantScopes: []string{".", "tools"},
			wantItems:  []string{"instruction:tools:GEMINI.md"},
		},
		{
			name:       "the configuration directory is never indexed",
			files:      map[string]string{".ai-rulez/skills/own/SKILL.md": skillMD("own", "Own"), ".ai-rulez/rules/a.md": ruleMD("A")},
			presets:    []string{"claude"},
			wantScopes: []string{"."},
		},
		{
			name:       "a skill inside another skill belongs to the outer one",
			files:      map[string]string{"skills/outer/SKILL.md": skillMD("outer", "Outer"), "skills/outer/examples/inner/SKILL.md": skillMD("inner", "Inner")},
			wantScopes: []string{"."},
			wantItems:  []string{"skill:.:outer"},
		},
		{
			name:       "rules and agents of every convention are indexed whatever the presets",
			files:      map[string]string{"pkg/.cursor/rules/ui.mdc": ruleMD("UI"), ".claude/rules/a.md": ruleMD("A"), ".claude/agents/b.md": ruleMD("B")},
			presets:    []string{"codex"},
			wantScopes: []string{".", "pkg"},
			wantItems:  []string{"agent:.:b", "rule:.:a", "rule:pkg:ui"},
		},
		{
			name:       "no presets (a lint-only configuration) still indexes rules and agents",
			files:      map[string]string{".claude/rules/a.md": ruleMD("A"), ".claude/agents/b.md": ruleMD("B")},
			wantScopes: []string{"."},
			wantItems:  []string{"agent:.:b", "rule:.:a"},
		},
		{
			name:       "a skills directory of a preset that is not enabled is ignored",
			files:      map[string]string{"pkg/.cursor/skills/ui/SKILL.md": skillMD("ui", "UI")},
			presets:    []string{"claude"},
			wantScopes: []string{"."},
		},
		{
			name:       "instruction files in hidden directories are not scopes",
			files:      map[string]string{".devcontainer/AGENTS.md": "# Dev\n"},
			wantScopes: []string{"."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := layout(t, tt.files)

			// Act
			ix := build(t, root, tt.presets...)

			// Assert
			assert.Equal(t, tt.wantScopes, scopePaths(ix))
			assert.Equal(t, tt.wantItems, nilIfEmpty(keys(ix.Items())))
		})
	}
}

func TestBuild_ExcludeGlobs(t *testing.T) {
	tests := []struct {
		name       string
		exclude    []string
		wantScopes []string
		wantItems  []string
	}{
		{
			name:       "a directory glob drops everything below it and its scope",
			exclude:    []string{"pkg/api/**"},
			wantScopes: []string{".", "pkg/web"},
			wantItems:  []string{"instruction:.:AGENTS.md", "instruction:.:CLAUDE.md", "rule:.:style", "rule:pkg/web:ui", "skill:.:lint", "skill:pkg/web:ui"},
		},
		{
			name:       "a directory named without a wildcard drops what is below it",
			exclude:    []string{"pkg"},
			wantScopes: []string{"."},
			wantItems:  []string{"instruction:.:AGENTS.md", "instruction:.:CLAUDE.md", "rule:.:style", "skill:.:lint"},
		},
		{
			name:       "a pattern without a slash matches at any depth",
			exclude:    []string{"AGENTS.md"},
			wantScopes: []string{".", "pkg/api", "pkg/web"},
			wantItems: []string{
				"agent:pkg/api:reviewer", "instruction:.:CLAUDE.md", "rule:.:style", "rule:pkg/web:ui",
				"skill:.:lint", "skill:pkg/api:lint", "skill:pkg/web:ui",
			},
		},
		{
			name:       "an extension glob",
			exclude:    []string{"**/*.mdc", "**/SKILL.md"},
			wantScopes: []string{".", "pkg/api"},
			wantItems: []string{
				"agent:pkg/api:reviewer", "instruction:.:AGENTS.md", "instruction:.:CLAUDE.md",
				"instruction:pkg/api:AGENTS.md", "rule:.:style",
			},
		},
		{
			name:       "braces and a trailing slash",
			exclude:    []string{"pkg/{api,web}/"},
			wantScopes: []string{"."},
			wantItems:  []string{"instruction:.:AGENTS.md", "instruction:.:CLAUDE.md", "rule:.:style", "skill:.:lint"},
		},
		{
			name:       "a pattern that matches nothing changes nothing",
			exclude:    []string{"elsewhere/**"},
			wantScopes: []string{".", "pkg/api", "pkg/web"},
			wantItems: []string{
				"agent:pkg/api:reviewer", "instruction:.:AGENTS.md", "instruction:.:CLAUDE.md", "instruction:pkg/api:AGENTS.md",
				"rule:.:style", "rule:pkg/web:ui", "skill:.:lint", "skill:pkg/api:lint", "skill:pkg/web:ui",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := layout(t, monorepo())

			// Act
			ix, err := Build(context.Background(), Options{Root: root, Presets: []string{"claude", "cursor"}, Exclude: tt.exclude})

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantScopes, scopePaths(ix))
			assert.Equal(t, tt.wantItems, nilIfEmpty(keys(ix.Items())))
		})
	}
}

func TestBuild_RulePathsAndGlobsAreIndexed(t *testing.T) {
	tests := []struct {
		name      string
		file      string
		front     string
		wantPaths []string
		wantGlobs []string
	}{
		{name: "paths as a list", file: ".claude/rules/a.md", front: "paths:\n  - \"src/**/*.ts\"\n  - lib/*.go\n", wantPaths: []string{"src/**/*.ts", "lib/*.go"}},
		{name: "paths as one string", file: ".claude/rules/a.md", front: "paths: \"src/**\"\n", wantPaths: []string{"src/**"}},
		{name: "paths as a comma separated string keeps braces whole", file: ".claude/rules/a.md", front: "paths: \"a/*.{ts,tsx}, b/**\"\n", wantPaths: []string{"a/*.{ts,tsx}", "b/**"}},
		{name: "cursor globs", file: ".cursor/rules/a.mdc", front: "globs: \"*.tsx,*.ts\"\n", wantGlobs: []string{"*.tsx", "*.ts"}},
		{name: "cursor globs as a list", file: ".cursor/rules/a.mdc", front: "globs:\n  - \"*.tsx\"\n", wantGlobs: []string{"*.tsx"}},
		{name: "cursor alwaysApply wins over globs", file: ".cursor/rules/a.mdc", front: "alwaysApply: true\nglobs: \"*.tsx\"\n"},
		{name: "no restriction", file: ".claude/rules/a.md", front: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := layout(t, map[string]string{"pkg/" + tt.file: "---\ndescription: A rule\n" + tt.front + "---\n\nBody.\n"})

			// Act
			ix := build(t, root)

			// Assert
			items := ix.Items()
			require.Len(t, items, 1)
			if tt.wantPaths == nil && tt.wantGlobs == nil {
				assert.Nil(t, items[0].Only)
				return
			}
			require.NotNil(t, items[0].Only)
			assert.Equal(t, tt.wantPaths, items[0].Only.Paths)
			assert.Equal(t, tt.wantGlobs, items[0].Only.Globs)
		})
	}
}

func TestBuild_RulePathsSurviveTheCache(t *testing.T) {
	root := layout(t, map[string]string{".claude/rules/a.md": "---\npaths: [\"src/**\"]\n---\n\nBody.\n"})
	cache := newFakeCache()
	_, err := Build(context.Background(), Options{Root: root, Cache: cache})
	require.NoError(t, err)
	cache.items = cache.stored

	ix, err := Build(context.Background(), Options{Root: root, Cache: cache})

	require.NoError(t, err)
	require.Len(t, ix.Items(), 1)
	require.NotNil(t, ix.Items()[0].Only)
	assert.Equal(t, []string{"src/**"}, ix.Items()[0].Only.Paths, "a cache hit keeps the paths")
}

func TestBuild_ExcludedFilesAreNeverRead(t *testing.T) {
	// Arrange: an excluded file that is over the size the index reads would be a problem if it were read.
	root := layout(t, map[string]string{
		"AGENTS.md":                 "# Root\n",
		"vendor/AGENTS.md":          strings.Repeat("x", MaxFileBytes+1),
		"vendor/.claude/rules/r.md": ruleMD("R"),
		"app/.claude/rules/keep.md": ruleMD("Keep"),
	})
	var read []string
	screen := func(it Item, _ []byte) (bool, string) { read = append(read, it.Path); return true, "" }

	// Act
	ix, err := Build(context.Background(), Options{Root: root, Exclude: []string{"vendor/**"}, Screen: screen})

	// Assert
	require.NoError(t, err)
	assert.Empty(t, ix.Problems(), "the oversized excluded file was not read")
	assert.ElementsMatch(t, []string{"AGENTS.md", "app/.claude/rules/keep.md"}, read)
}

func TestBuild_RejectsAnInvalidExcludeGlob(t *testing.T) {
	root := layout(t, monorepo())

	_, err := Build(context.Background(), Options{Root: root, Exclude: []string{"{a,b}{a,b}{a,b}{a,b}{a,b}{a,b}{a,b}"}})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "exclude")
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

func TestBuild_ItemFields(t *testing.T) {
	// Arrange
	root := layout(t, map[string]string{
		"AGENTS.md":                    "# Root conventions\n\nBe terse.\n",
		".claude/skills/lint/SKILL.md": skillMD("lint", "Lint the root"),
		".claude/rules/style.md":       ruleMD("Style rules"),
		"pkg/AGENTS.md":                "---\ndescription: From frontmatter\n---\n\n# Heading\n",
		"pkg/CLAUDE.md":                "no heading here\n",
	})

	// Act
	ix := build(t, root, "claude")

	// Assert
	byKey := map[string]Item{}
	for _, it := range ix.Items() {
		byKey[string(it.Kind)+":"+it.Scope+":"+it.Name] = it
	}
	tests := []struct {
		key         string
		source      string
		description string
		path        string
	}{
		{"instruction:.:AGENTS.md", "agents", "Root conventions", "AGENTS.md"},
		{"skill:.:lint", "claude", "Lint the root", ".claude/skills/lint"},
		{"rule:.:style", "claude", "Style rules", ".claude/rules/style.md"},
		{"instruction:pkg:AGENTS.md", "agents", "From frontmatter", "pkg/AGENTS.md"},
		{"instruction:pkg:CLAUDE.md", "claude", "", "pkg/CLAUDE.md"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			it, ok := byKey[tt.key]
			require.True(t, ok, "item %s is indexed", tt.key)
			assert.Equal(t, tt.source, it.Source)
			assert.Equal(t, tt.description, it.Description)
			assert.Equal(t, tt.path, it.Path)
			assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, it.Hash)
		})
	}
	assert.NotEqual(t, byKey["instruction:.:AGENTS.md"].Hash, byKey["instruction:pkg:CLAUDE.md"].Hash)
}

func TestBuild_HashFollowsTheContent(t *testing.T) {
	// Arrange
	root := layout(t, map[string]string{"AGENTS.md": "# One\n"})
	before := build(t, root).Items()

	// Act
	require.NoError(t, os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Two\n"), 0o644))
	after := build(t, root).Items()

	// Assert
	require.Len(t, before, 1)
	require.Len(t, after, 1)
	assert.NotEqual(t, before[0].Hash, after[0].Hash)
	assert.Equal(t, "Two", after[0].Description)
}

func TestBuild_DescriptionIsBounded(t *testing.T) {
	// Arrange
	root := layout(t, map[string]string{"AGENTS.md": "# " + strings.Repeat("long ", 400) + "\n"})

	// Act
	items := build(t, root).Items()

	// Assert
	require.Len(t, items, 1)
	assert.LessOrEqual(t, len(items[0].Description), 500)
}

func TestBuild_LeavesOutWhatItShouldNot(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, root string)
		generated func(path string, content []byte) bool
		skip      []string
		want      []string
	}{
		{
			name: "a symlinked instruction file is not followed",
			setup: func(t *testing.T, root string) {
				outside := filepath.Join(t.TempDir(), "secret.md")
				require.NoError(t, os.WriteFile(outside, []byte("# Secret\n"), 0o644))
				require.NoError(t, os.MkdirAll(filepath.Join(root, "pkg"), 0o755))
				testutil.SymlinkOrSkip(t, outside, filepath.Join(root, "pkg", "AGENTS.md"))
			},
		},
		{
			name: "a symlinked directory is not entered",
			setup: func(t *testing.T, root string) {
				outside := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(outside, "AGENTS.md"), []byte("# Elsewhere\n"), 0o644))
				testutil.SymlinkOrSkip(t, outside, filepath.Join(root, "linked"))
			},
		},
		{
			name: "a generated copy is skipped",
			setup: func(t *testing.T, root string) {
				testutil.WriteTree(t, root, map[string]string{"CLAUDE.md": "generated\n", "pkg/AGENTS.md": "# Mine\n"})
			},
			generated: func(path string, _ []byte) bool { return filepath.Base(path) == "CLAUDE.md" },
			want:      []string{"instruction:pkg:AGENTS.md"},
		},
		{
			name: "a skipped prefix is not entered",
			setup: func(t *testing.T, root string) {
				testutil.WriteTree(t, root, map[string]string{"vendor/lib/AGENTS.md": "# Vendored\n", "pkg/AGENTS.md": "# Mine\n"})
			},
			skip: []string{"vendor"},
			want: []string{"instruction:pkg:AGENTS.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			tt.setup(t, root)

			// Act
			ix, err := Build(context.Background(), Options{Root: root, Skip: tt.skip, Generated: tt.generated})

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, nilIfEmpty(keys(ix.Items())))
		})
	}
}

func TestBuild_CountsGeneratedSkillsAndKeepsTheirRoot(t *testing.T) {
	// Arrange
	root := layout(t, map[string]string{
		".claude/skills/gen/SKILL.md":  skillMD("gen", "Written by ai-rulez"),
		".claude/skills/mine/SKILL.md": skillMD("mine", "Written by hand"),
	})
	generated := func(path string, _ []byte) bool { return strings.Contains(filepath.ToSlash(path), "/gen/") }

	// Act
	ix, err := Build(context.Background(), Options{Root: root, Presets: []string{"claude"}, Generated: generated})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"skill:.:mine"}, keys(ix.Items()))
	assert.Equal(t, 1, ix.GeneratedSkipped(KindSkill))
	assert.Equal(t, []string{".claude/skills"}, ix.SkillRoots())
}

func TestBuild_SkillsComeShallowestFirst(t *testing.T) {
	// Arrange
	root := layout(t, map[string]string{
		"b/deep/er/skills/z/SKILL.md": skillMD("z", "Z"),
		"skills/a/SKILL.md":           skillMD("a", "A"),
		"b/skills/m/SKILL.md":         skillMD("m", "M"),
		"a/skills/m/SKILL.md":         skillMD("m", "M2"),
	})

	// Act
	skills := build(t, root).Skills()

	// Assert
	var paths []string
	for _, s := range skills {
		paths = append(paths, s.Path)
	}
	assert.Equal(t, []string{"skills/a", "a/skills/m", "b/skills/m", "b/deep/er/skills/z"}, paths)
}

func TestBuild_ListsGitFilesNotIgnoredOnes(t *testing.T) {
	// Arrange
	root := layout(t, map[string]string{
		".gitignore":        "ignored/\n",
		"AGENTS.md":         "# Tracked\n",
		"ignored/AGENTS.md": "# Ignored\n",
		"fresh/AGENTS.md":   "# Untracked but not ignored\n",
	})
	testutil.Git(t, root, "init", "--quiet")
	testutil.Git(t, root, "add", ".gitignore", "AGENTS.md")

	// Act
	ix := build(t, root)

	// Assert
	assert.Equal(t, []string{"instruction:.:AGENTS.md", "instruction:fresh:AGENTS.md"}, keys(ix.Items()))
}

func TestBuild_WalksWhenThereIsNoGit(t *testing.T) {
	// Arrange
	root := layout(t, map[string]string{"AGENTS.md": "# Root\n", "pkg/AGENTS.md": "# Pkg\n", "node_modules/x/AGENTS.md": "# Dep\n", ".git/AGENTS.md": "# Git\n"})

	// Act
	ix := build(t, root)

	// Assert
	assert.Equal(t, []string{"instruction:.:AGENTS.md", "instruction:pkg:AGENTS.md"}, keys(ix.Items()))
}

func TestBuild_CapsTheIndexKeepingShallowScopes(t *testing.T) {
	// Arrange
	files := map[string]string{"AGENTS.md": "# Root\n"}
	for _, dir := range []string{"a", "b", "c/d", "e/f/g"} {
		files[dir+"/AGENTS.md"] = "# " + dir + "\n"
	}
	root := layout(t, files)

	// Act
	ix, err := Build(context.Background(), Options{Root: root, MaxItems: 3})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 5, ix.Found())
	assert.Equal(t, 3, ix.Limit())
	assert.True(t, ix.Truncated())
	assert.Equal(t, []string{"instruction:.:AGENTS.md", "instruction:a:AGENTS.md", "instruction:b:AGENTS.md"}, keys(ix.Items()))
}

func TestBuild_DefaultCapIsTwentyThousand(t *testing.T) {
	ix := build(t, layout(t, map[string]string{"AGENTS.md": "# Root\n"}))

	assert.Equal(t, 20000, ix.Limit())
	assert.False(t, ix.Truncated())
}

func TestBuild_ScreenDropsOrRefusesItems(t *testing.T) {
	// Arrange
	root := layout(t, map[string]string{
		"AGENTS.md":           "# Keep\n",
		"a/AGENTS.md":         "# Silently dropped\n",
		"b/AGENTS.md":         "# Refused for a reason\n",
		"b/skills/s/SKILL.md": skillMD("s", "A skill"),
	})
	screen := func(it Item, content []byte) (bool, string) {
		switch {
		case it.Kind != KindInstruction:
			return true, ""
		case it.Scope == "a":
			return false, ""
		case it.Scope == "b":
			assert.Contains(t, string(content), "Refused", "the screen sees the content")
			return false, "blocked by the scan"
		}
		return true, ""
	}

	// Act
	ix, err := Build(context.Background(), Options{Root: root, Screen: screen})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"instruction:.:AGENTS.md", "skill:b:s"}, keys(ix.Items()))
	require.Len(t, ix.Refusals(), 1)
	assert.Equal(t, "b", ix.Refusals()[0].Item.Scope)
	assert.Equal(t, "blocked by the scan", ix.Refusals()[0].Reason)
}

func TestBuild_CollidingNamesInAScopeAreNamespacedByPath(t *testing.T) {
	// Arrange
	root := layout(t, map[string]string{
		"pkg/.claude/agents/review.md": ruleMD("Agent"),
		"pkg/.claude/rules/review.md":  ruleMD("Rule"),
		"pkg/.claude/rules/other.md":   ruleMD("Other"),
	})

	// Act
	ix := build(t, root, "claude")

	// Assert
	assert.Equal(t, []string{"agent:pkg:review", "rule:pkg:other", "rule:pkg:pkg.claude.rules.review"}, keys(ix.Items()))
	got, ok := ix.Lookup("pkg", "pkg.claude.rules.review")
	require.True(t, ok)
	assert.Equal(t, "pkg/.claude/rules/review.md", got.Path)
}

func TestBuild_AnUnreadableOrOversizedFileIsAProblemNotAnError(t *testing.T) {
	// Arrange
	root := layout(t, map[string]string{
		"AGENTS.md":     "# Fine\n",
		"big/AGENTS.md": strings.Repeat("x", 2<<20),
	})

	// Act
	ix := build(t, root)

	// Assert
	assert.Equal(t, []string{"instruction:.:AGENTS.md"}, keys(ix.Items()))
	require.Len(t, ix.Problems(), 1)
	assert.Equal(t, "big/AGENTS.md", ix.Problems()[0].Path)
}

func TestIndex_Lookup(t *testing.T) {
	ix := build(t, layout(t, monorepo()), "claude", "cursor")

	tests := []struct {
		name, scope, item string
		ok                bool
	}{
		{"instruction", "pkg/api", "AGENTS.md", true},
		{"rule", ".", "style", true},
		{"unknown scope", "nope", "AGENTS.md", false},
		{"unknown name", "pkg/api", "nope", false},
		{"skills are not looked up by scope and name here", ".", "lint", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := ix.Lookup(tt.scope, tt.item)

			assert.Equal(t, tt.ok, ok)
		})
	}
}

func TestIndex_ScopeSummary(t *testing.T) {
	ix := build(t, layout(t, monorepo()), "claude", "cursor")

	api, ok := ix.Scope("pkg/api")

	require.True(t, ok)
	assert.Equal(t, "pkg/api", api.Path)
	assert.Equal(t, map[Kind]int{KindInstruction: 1, KindAgent: 1, KindSkill: 1}, api.Counts)
	assert.Equal(t, []string{"agents", "claude"}, api.Sources)
	_, ok = ix.Scope("pkg/none")
	assert.False(t, ok)
}

func TestBuild_SkillPathsAreIndexed(t *testing.T) {
	tests := []struct {
		name      string
		front     string
		wantPaths []string
	}{
		{name: "paths as a list", front: "paths:\n  - \"src/**/*.ts\"\n  - lib/*.go\n", wantPaths: []string{"src/**/*.ts", "lib/*.go"}},
		{name: "paths as one string", front: "paths: \"src/**\"\n", wantPaths: []string{"src/**"}},
		{name: "paths as a comma separated string keeps braces whole", front: "paths: \"a/*.{ts,tsx}, b/**\"\n", wantPaths: []string{"a/*.{ts,tsx}", "b/**"}},
		{name: "cursor globs do not apply to a skill", front: "globs: \"*.tsx\"\n"},
		{name: "no restriction", front: ""},
		{name: "empty paths", front: "paths: []\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := layout(t, map[string]string{"pkg/.claude/skills/a/SKILL.md": "---\nname: a\ndescription: A skill\n" + tt.front + "---\n\nBody.\n"})

			// Act
			ix := build(t, root)

			// Assert
			items := ix.Items()
			require.Len(t, items, 1)
			require.Equal(t, KindSkill, items[0].Kind)
			if tt.wantPaths == nil {
				assert.Nil(t, items[0].Only)
				return
			}
			require.NotNil(t, items[0].Only)
			assert.Equal(t, tt.wantPaths, items[0].Only.Paths)
			assert.Empty(t, items[0].Only.Globs)
		})
	}
}

func TestBuild_SkillPathsSurviveTheCache(t *testing.T) {
	root := layout(t, map[string]string{".claude/skills/a/SKILL.md": "---\nname: a\ndescription: A\npaths: [\"src/**\"]\n---\n\nBody.\n"})
	cache := newFakeCache()
	_, err := Build(context.Background(), Options{Root: root, Cache: cache})
	require.NoError(t, err)
	cache.items = cache.stored

	ix, err := Build(context.Background(), Options{Root: root, Cache: cache})

	require.NoError(t, err)
	require.Len(t, ix.Items(), 1)
	require.NotNil(t, ix.Items()[0].Only)
	assert.Equal(t, []string{"src/**"}, ix.Items()[0].Only.Paths, "a cache hit keeps the paths")
}
