package generator

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	bazScopedRule = "---\npaths:\n  - services/api/**/*.py\n---\n# API Style\n\nAPI_STYLE_BODY\n"
	bazRootRule   = "---\npaths:\n  - \"**/*.sql\"\n---\n# Sql Style\n\nSQL_STYLE_BODY\n"
	// bazTargetedRule is path-scoped and names baz in targets, so it reaches the
	// nested surface even under rules.baz_default = "none".
	bazTargetedRule = "---\npaths:\n  - services/db/**/*.py\ntargets: [baz]\n---\n# DB Style\n\nDB_STYLE_BODY\n"
	// bazSiblingRule has two globs pointing into sibling directories under
	// services, so rules.baz_dedupe can collapse them to one shared parent file.
	bazSiblingRule = "---\npaths:\n  - services/api/**/*.py\n  - services/web/**/*.py\n---\n# Sibling\n\nSIBLING_BODY\n"
)

func bazProjectFiles(t *testing.T, root, cfgTOML string) {
	t.Helper()
	writeAgentsMDProject(t, root, cfgTOML)
	writeAgentsMDFile(t, root, ".ai-rulez/rules/api-style.md", bazScopedRule)
	writeAgentsMDFile(t, root, ".ai-rulez/rules/sql-style.md", bazRootRule)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "services", "api"), 0o755))
}

func readBazFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	require.NoError(t, err, rel)
	return string(data)
}

func TestBaz_ComposesWithClaudeAndCodex(t *testing.T) {
	tests := []struct {
		name    string
		presets []string
		flag    string
	}{
		{"baz alone", []string{"baz"}, ""},
		{"baz and claude", []string{"claude", "baz"}, ""},
		{"baz and claude with agents_md", []string{"claude", "baz"}, "agents_md = true\n"},
		{"baz, claude and codex", []string{"claude", "codex", "baz"}, ""},
		{"baz, claude and codex with agents_md", []string{"claude", "codex", "baz"}, "agents_md = true\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			bazProjectFiles(t, root, agentsMDConfig(tt.presets, tt.flag, ""))
			runAgentsMDGenerate(t, root)

			rootMD := readBazFile(t, root, "AGENTS.md")
			assert.Contains(t, rootMD, "ALWAYS_BODY")
			assert.Contains(t, rootMD, "SQL_STYLE_BODY", "a glob without a directory stays in the root file")
			assert.NotContains(t, rootMD, "API_STYLE_BODY", "the scoped rule moved to its directory")

			nested := readBazFile(t, root, "services/api/AGENTS.md")
			assert.Contains(t, nested, "API_STYLE_BODY")
			assert.NotContains(t, nested, "ALWAYS_BODY")

			// Commands and .claude/rules are not something Baz reads; baz adds neither.
			_, err := os.Stat(filepath.Join(root, ".agents", "commands"))
			assert.True(t, os.IsNotExist(err))

			claude, codex := false, false
			for _, p := range tt.presets {
				claude = claude || p == "claude"
				codex = codex || p == "codex"
			}
			_, skillsErr := os.Stat(filepath.Join(root, ".agents", "skills", "alpha", "SKILL.md"))
			_, claudeSkillsErr := os.Stat(filepath.Join(root, ".claude", "skills", "alpha", "SKILL.md"))
			if claude && tt.flag == "" {
				assert.NoError(t, claudeSkillsErr)
				if codex {
					assert.NoError(t, skillsErr, "codex writes the skill to .agents/skills")
				} else {
					assert.True(t, os.IsNotExist(skillsErr), "claude already provides the skill at .claude/skills")
				}
			}
		})
	}
}

func TestBaz_ClaudeOutputUnchangedByBaz(t *testing.T) {
	without := t.TempDir()
	bazProjectFiles(t, without, agentsMDConfig([]string{"claude"}, "", ""))
	runAgentsMDGenerate(t, without)

	with := t.TempDir()
	bazProjectFiles(t, with, agentsMDConfig([]string{"claude", "baz"}, "", ""))
	runAgentsMDGenerate(t, with)

	for _, rel := range []string{".claude/skills/alpha/SKILL.md", ".claude/agents/helper.md", ".claude/rules/always.md"} {
		assert.Equal(t, stripHeaderHashes(readBazFile(t, without, rel)), stripHeaderHashes(readBazFile(t, with, rel)), rel)
	}
}

// TestBaz_ClaudeSurfaceUnchangedInEveryMode pins that no baz mode writes into
// the Claude tree: the .claude files are the same with and without baz.
func TestBaz_ClaudeSurfaceUnchangedInEveryMode(t *testing.T) {
	without := t.TempDir()
	bazProjectFiles(t, without, agentsMDConfig([]string{"claude"}, "", ""))
	runAgentsMDGenerate(t, without)
	var claudeFiles []string
	for rel := range agentsMDSnapshot(t, without) {
		if strings.HasPrefix(rel, ".claude/") {
			claudeFiles = append(claudeFiles, rel)
		}
	}
	require.NotEmpty(t, claudeFiles)

	modes := []struct{ name, extra string }{
		{"default", ""},
		{"scoped root", "\n[rules]\nbaz_scoped = \"root\"\n"},
		{"baz_default none", "\n[rules]\nbaz_default = \"none\"\n"},
		{"baz_dedupe", "\n[rules]\nbaz_dedupe = true\n"},
		{"baz_budget", "\n[rules]\nbaz_budget = 1\n"},
	}
	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			with := t.TempDir()
			bazProjectFiles(t, with, agentsMDConfig([]string{"claude", "baz"}, "", m.extra))
			runAgentsMDGenerate(t, with)
			for _, rel := range claudeFiles {
				assert.Equal(t, stripHeaderHashes(readBazFile(t, without, rel)),
					stripHeaderHashes(readBazFile(t, with, rel)), rel)
			}
		})
	}
}

func TestBaz_RootModeKeepsScopedRuleInRootFile(t *testing.T) {
	root := t.TempDir()
	bazProjectFiles(t, root, agentsMDConfig([]string{"baz"}, "", "\n[rules]\nbaz_scoped = \"root\"\n"))
	runAgentsMDGenerate(t, root)

	assert.Contains(t, readBazFile(t, root, "AGENTS.md"), "API_STYLE_BODY")
	_, err := os.Stat(filepath.Join(root, "services", "api", "AGENTS.md"))
	assert.True(t, os.IsNotExist(err))
}

func TestBaz_StaleNestedFileIsRemovedWhenRuleGoes(t *testing.T) {
	root := t.TempDir()
	bazProjectFiles(t, root, agentsMDConfig([]string{"baz"}, "", ""))
	runAgentsMDGenerate(t, root)
	require.FileExists(t, filepath.Join(root, "services", "api", "AGENTS.md"))

	require.NoError(t, os.Remove(filepath.Join(root, ".ai-rulez", "rules", "api-style.md")))
	runAgentsMDGenerate(t, root)
	assert.NoFileExists(t, filepath.Join(root, "services", "api", "AGENTS.md"))
}

func TestBaz_NeverWritesIntoTheConfigDirectory(t *testing.T) {
	root := t.TempDir()
	bazProjectFiles(t, root, agentsMDConfig([]string{"baz"}, "", ""))
	writeAgentsMDFile(t, root, ".ai-rulez/rules/self.md", "---\npaths:\n  - .ai-rulez/rules/**\n---\n# Self\n\nSELF_BODY\n")
	runAgentsMDGenerate(t, root)
	runAgentsMDGenerate(t, root)

	assert.NoFileExists(t, filepath.Join(root, ".ai-rulez", "rules", "AGENTS.md"))
	assert.Contains(t, readBazFile(t, root, "AGENTS.md"), "SELF_BODY")
}

var headerHashLine = regexp.MustCompile(`(?m)^#? ?(Content|Source)-Hash:.*$`)

func stripHeaderHashes(s string) string {
	return headerHashLine.ReplaceAllString(s, "")
}

// assertFilePresence asserts that path exists exactly when want is true.
func assertFilePresence(t *testing.T, path string, want bool) {
	t.Helper()
	if want {
		assert.FileExists(t, path)
		return
	}
	assert.NoFileExists(t, path)
}

// TestBaz_DefaultNoneNestsOnlyBazTargetedItems pins rules.baz_default. The
// default ("all") nests every path-scoped item; "none" nests only the items
// whose frontmatter targets name baz and leaves the others in the root file.
func TestBaz_DefaultNoneNestsOnlyBazTargetedItems(t *testing.T) {
	tests := []struct {
		name        string
		extra       string
		wantAPIFile bool
		wantDBFile  bool
		wantRootAPI bool
		wantRootDB  bool
	}{
		{
			name:        "default nests every item",
			wantAPIFile: true, wantDBFile: true,
			wantRootAPI: false, wantRootDB: false,
		},
		{
			name:        "none nests only the baz-targeted item",
			extra:       "\n[rules]\nbaz_default = \"none\"\n",
			wantAPIFile: false, wantDBFile: true,
			wantRootAPI: true, wantRootDB: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeAgentsMDProject(t, root, agentsMDConfig([]string{"baz"}, "", tt.extra))
			writeAgentsMDFile(t, root, ".ai-rulez/rules/api-style.md", bazScopedRule)
			writeAgentsMDFile(t, root, ".ai-rulez/rules/db-style.md", bazTargetedRule)
			require.NoError(t, os.MkdirAll(filepath.Join(root, "services", "api"), 0o755))
			require.NoError(t, os.MkdirAll(filepath.Join(root, "services", "db"), 0o755))
			runAgentsMDGenerate(t, root)

			assertFilePresence(t, filepath.Join(root, "services", "api", "AGENTS.md"), tt.wantAPIFile)
			assertFilePresence(t, filepath.Join(root, "services", "db", "AGENTS.md"), tt.wantDBFile)
			rootMD := readBazFile(t, root, "AGENTS.md")
			assert.Equal(t, tt.wantRootAPI, strings.Contains(rootMD, "API_STYLE_BODY"), "untargeted rule in root")
			assert.Equal(t, tt.wantRootDB, strings.Contains(rootMD, "DB_STYLE_BODY"), "baz-targeted rule in root")
		})
	}
}

// TestBaz_DedupeSharesParentFile pins rules.baz_dedupe. Off (the default) writes
// one nested file per glob directory; on writes a single shared parent file.
func TestBaz_DedupeSharesParentFile(t *testing.T) {
	tests := []struct {
		name       string
		extra      string
		wantAPI    bool
		wantWeb    bool
		wantParent bool
	}{
		{
			name:    "off writes one file per directory",
			wantAPI: true, wantWeb: true, wantParent: false,
		},
		{
			name:       "on writes the shared parent",
			extra:      "\n[rules]\nbaz_dedupe = true\n",
			wantAPI:    false,
			wantWeb:    false,
			wantParent: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeAgentsMDProject(t, root, agentsMDConfig([]string{"baz"}, "", tt.extra))
			writeAgentsMDFile(t, root, ".ai-rulez/rules/sibling.md", bazSiblingRule)
			require.NoError(t, os.MkdirAll(filepath.Join(root, "services", "api"), 0o755))
			require.NoError(t, os.MkdirAll(filepath.Join(root, "services", "web"), 0o755))
			runAgentsMDGenerate(t, root)

			assertFilePresence(t, filepath.Join(root, "services", "api", "AGENTS.md"), tt.wantAPI)
			assertFilePresence(t, filepath.Join(root, "services", "web", "AGENTS.md"), tt.wantWeb)
			assertFilePresence(t, filepath.Join(root, "services", "AGENTS.md"), tt.wantParent)
			assert.NotContains(t, readBazFile(t, root, "AGENTS.md"), "SIBLING_BODY", "the item is nested, not inlined")
			if tt.wantParent {
				assert.Contains(t, readBazFile(t, root, "services/AGENTS.md"), "SIBLING_BODY")
			}
		})
	}
}

// TestBaz_DedupeOffMatchesExplicitOff pins that the default output is the same
// whether rules.baz_dedupe is absent or set to false: the new mode is opt-in.
func TestBaz_DedupeOffMatchesExplicitOff(t *testing.T) {
	build := func(t *testing.T, extra string) map[string]string {
		t.Helper()
		root := t.TempDir()
		writeAgentsMDProject(t, root, agentsMDConfig([]string{"baz"}, "", extra))
		writeAgentsMDFile(t, root, ".ai-rulez/rules/sibling.md", bazSiblingRule)
		require.NoError(t, os.MkdirAll(filepath.Join(root, "services", "api"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "services", "web"), 0o755))
		runAgentsMDGenerate(t, root)
		return agentsMDSnapshot(t, root)
	}

	absent := build(t, "")
	explicitOff := build(t, "\n[rules]\nbaz_dedupe = false\n")
	assert.Equal(t, absent, explicitOff)
}

// TestBaz_BudgetFinding pins rules.baz_budget: the finding is raised above the
// budget, names the files and the budget, and is absent below it or when the
// warning is off.
func TestBaz_BudgetFinding(t *testing.T) {
	build := func(t *testing.T, extra string) *bazBudgetFinding {
		t.Helper()
		root := t.TempDir()
		writeAgentsMDProject(t, root, agentsMDConfig([]string{"baz"}, "", extra))
		writeAgentsMDFile(t, root, ".ai-rulez/rules/api-style.md", bazScopedRule)
		require.NoError(t, os.MkdirAll(filepath.Join(root, "services", "api"), 0o755))
		cfg, err := config.LoadConfig(context.Background(), root)
		require.NoError(t, err)
		g := NewGenerator(cfg)
		outputs, _, err := g.collectOutputs("")
		require.NoError(t, err)
		return g.bazBudgetFinding(outputs)
	}

	t.Run("above the budget warns", func(t *testing.T) {
		f := build(t, "\n[rules]\nbaz_budget = 1\n")
		require.NotNil(t, f)
		assert.Equal(t, 1, f.Budget)
		assert.Greater(t, f.Bytes, f.Budget)
		require.NotEmpty(t, f.Files)
		msg := f.message()
		assert.Contains(t, msg, "rules.baz_budget")
		assert.Contains(t, msg, f.Files[0].Label, "the message names a file")
		assert.Contains(t, msg, "budget 1")
	})

	t.Run("below the budget is silent", func(t *testing.T) {
		assert.Nil(t, build(t, "\n[rules]\nbaz_budget = 1048576\n"))
	})

	t.Run("off is silent", func(t *testing.T) {
		assert.Nil(t, build(t, ""))
	})

	t.Run("only with baz configured", func(t *testing.T) {
		root := t.TempDir()
		writeAgentsMDProject(t, root, agentsMDConfig([]string{"codex"}, "", "\n[rules]\nbaz_budget = 1\n"))
		cfg, err := config.LoadConfig(context.Background(), root)
		require.NoError(t, err)
		g := NewGenerator(cfg)
		outputs, _, err := g.collectOutputs("")
		require.NoError(t, err)
		assert.Nil(t, g.bazBudgetFinding(outputs))
	})
}

// TestBaz_PerRuleOptIn pins the three per-rule controls: `!baz` in targets
// (deny wins, even under baz_default = "all"), `[rules.baz] include`, and
// `baz: true` frontmatter. The last two select an item under baz_default = "none".
func TestBaz_PerRuleOptIn(t *testing.T) {
	const (
		apiRule  = "---\npaths:\n  - services/api/**/*.py\n---\n# API Style\n\nAPI_STYLE_BODY\n"
		dbRule   = "---\npaths:\n  - services/db/**/*.py\n---\n# DB Style\n\nDB_STYLE_BODY\n"
		webRule  = "---\npaths:\n  - services/web/**/*.py\nbaz: true\n---\n# Web Style\n\nWEB_STYLE_BODY\n"
		skipRule = "---\npaths:\n  - services/skip/**/*.py\ntargets: [\"!baz\"]\n---\n# Skip Style\n\nSKIP_STYLE_BODY\n"
	)
	run := func(t *testing.T, extra string) string {
		t.Helper()
		root := t.TempDir()
		writeAgentsMDProject(t, root, agentsMDConfig([]string{"baz"}, "", extra))
		writeAgentsMDFile(t, root, ".ai-rulez/rules/api-style.md", apiRule)
		writeAgentsMDFile(t, root, ".ai-rulez/rules/db-style.md", dbRule)
		writeAgentsMDFile(t, root, ".ai-rulez/rules/web-style.md", webRule)
		writeAgentsMDFile(t, root, ".ai-rulez/rules/skip-style.md", skipRule)
		for _, d := range []string{"api", "db", "web", "skip"} {
			require.NoError(t, os.MkdirAll(filepath.Join(root, "services", d), 0o755))
		}
		runAgentsMDGenerate(t, root)
		return root
	}
	nested := func(root string) map[string]bool {
		out := map[string]bool{}
		for _, d := range []string{"api", "db", "web", "skip"} {
			_, err := os.Stat(filepath.Join(root, "services", d, "AGENTS.md"))
			out[d] = err == nil
		}
		return out
	}

	t.Run("negation excludes the rule under the default", func(t *testing.T) {
		root := run(t, "")
		assert.Equal(t, map[string]bool{"api": true, "db": true, "web": true, "skip": false}, nested(root))
		assert.Contains(t, readBazFile(t, root, "AGENTS.md"), "SKIP_STYLE_BODY", "an excluded rule stays in the shared root file")
	})
	t.Run("include and frontmatter opt in under baz_default none", func(t *testing.T) {
		root := run(t, "\n[rules]\nbaz_default = \"none\"\n\n[rules.baz]\ninclude = [\"db-style\"]\n")
		assert.Equal(t, map[string]bool{"api": false, "db": true, "web": true, "skip": false}, nested(root))
	})
	t.Run("deny beats include", func(t *testing.T) {
		root := run(t, "\n[rules]\nbaz_default = \"none\"\n\n[rules.baz]\ninclude = [\"skip-style\"]\n")
		assert.False(t, nested(root)["skip"])
	})
}

// TestBaz_UnscopedSkip pins rules.baz_unscoped. A glob-activated rule with no
// directory (**/*.go) cannot be nested, so the baz preset inlines it into the
// root AGENTS.md ("root", the default). "skip" drops it from that file, but only
// when no other preset shares the file: codex and friends still need it.
func TestBaz_UnscopedSkip(t *testing.T) {
	const goRule = "---\npaths:\n  - \"**/*.go\"\n---\n# Go Code\n\nGO_CODE_BODY\n"
	root := func(t *testing.T, presets []string, extra string) string {
		t.Helper()
		dir := t.TempDir()
		writeAgentsMDProject(t, dir, agentsMDConfig(presets, "", extra))
		writeAgentsMDFile(t, dir, ".ai-rulez/rules/go-code.md", goRule)
		writeAgentsMDFile(t, dir, ".ai-rulez/rules/api-style.md", bazScopedRule)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "services", "api"), 0o755))
		runAgentsMDGenerate(t, dir)
		return readBazFile(t, dir, "AGENTS.md")
	}
	skip := "\n[rules]\nbaz_unscoped = \"skip\"\n"

	t.Run("root is the default", func(t *testing.T) {
		assert.Contains(t, root(t, []string{"claude", "baz"}, ""), "GO_CODE_BODY")
		assert.Contains(t, root(t, []string{"claude", "baz"}, "\n[rules]\nbaz_unscoped = \"root\"\n"), "GO_CODE_BODY")
	})
	t.Run("skip drops it from the baz-only root file", func(t *testing.T) {
		md := root(t, []string{"claude", "baz"}, skip)
		assert.NotContains(t, md, "GO_CODE_BODY")
		assert.Contains(t, md, "ALWAYS_BODY", "unconditional rules stay")
		assert.NotContains(t, md, "API_STYLE_BODY", "nestable rules still move to their directory")
	})
	t.Run("skip leaves a root file shared with codex alone", func(t *testing.T) {
		assert.Contains(t, root(t, []string{"codex", "baz"}, skip), "GO_CODE_BODY")
	})
	t.Run("skip does not apply when baz_scoped is root", func(t *testing.T) {
		md := root(t, []string{"baz"}, "\n[rules]\nbaz_scoped = \"root\"\nbaz_unscoped = \"skip\"\n")
		assert.Contains(t, md, "GO_CODE_BODY")
	})
}

// TestBaz_BudgetWarningNamesInlinedUnscopedRules pins that the baz_budget
// warning names the glob-activated rules the root file inlines because they
// cannot be nested, and that they are not reported once skipped.
func TestBaz_BudgetWarningNamesInlinedUnscopedRules(t *testing.T) {
	const goRule = "---\npaths:\n  - \"**/*.go\"\n---\n# Go Code\n\nGO_CODE_BODY\n"
	names := func(t *testing.T, extra string) []string {
		t.Helper()
		dir := t.TempDir()
		writeAgentsMDProject(t, dir, agentsMDConfig([]string{"claude", "baz"}, "", extra))
		writeAgentsMDFile(t, dir, ".ai-rulez/rules/go-code.md", goRule)
		cfg, err := config.LoadConfig(context.Background(), dir)
		require.NoError(t, err)
		g := NewGenerator(cfg)
		outputs, _, err := g.collectOutputs("")
		require.NoError(t, err)
		f := g.bazBudgetFinding(outputs)
		require.NotNil(t, f)
		return f.Unscoped
	}
	assert.Equal(t, []string{"go-code"}, names(t, "\n[rules]\nbaz_budget = 1\n"))
	assert.Empty(t, names(t, "\n[rules]\nbaz_budget = 1\nbaz_unscoped = \"skip\"\n"))
}

// TestBaz_DedupeMax pins rules.baz_dedupe_max: a rule that would be copied into
// more than K directories collapses to the directory covering all of them, up to
// the repository root (the root AGENTS.md); at or below K the copies stay.
func TestBaz_DedupeMax(t *testing.T) {
	const (
		threeRule = "---\npaths:\n  - platform/**/*.proto\n  - deploy/**/*.proto\n  - dev_tools/**/*.proto\n---\n# Proto\n\nPROTO_BODY\n"
		siblings  = "---\npaths:\n  - svc/a/**/*.py\n  - svc/b/**/*.py\n  - svc/c/**/*.py\n---\n# Sib\n\nSIB_BODY\n"
	)
	run := func(t *testing.T, rule, extra string) string {
		t.Helper()
		root := t.TempDir()
		writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude", "baz"}, "", extra))
		writeAgentsMDFile(t, root, ".ai-rulez/rules/r.md", rule)
		for _, d := range []string{"platform", "deploy", "dev_tools", "svc/a", "svc/b", "svc/c"} {
			require.NoError(t, os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755))
		}
		runAgentsMDGenerate(t, root)
		return root
	}
	exists := func(root, rel string) bool {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		return err == nil
	}

	t.Run("off keeps one copy per directory", func(t *testing.T) {
		root := run(t, threeRule, "")
		assert.True(t, exists(root, "platform/AGENTS.md") && exists(root, "deploy/AGENTS.md") && exists(root, "dev_tools/AGENTS.md"))
	})
	t.Run("above K with no shared parent places it in the root file", func(t *testing.T) {
		root := run(t, threeRule, "\n[rules]\nbaz_dedupe_max = 2\n")
		assert.False(t, exists(root, "platform/AGENTS.md"))
		assert.Contains(t, readBazFile(t, root, "AGENTS.md"), "PROTO_BODY")
	})
	t.Run("at K keeps the copies", func(t *testing.T) {
		root := run(t, threeRule, "\n[rules]\nbaz_dedupe_max = 3\n")
		assert.True(t, exists(root, "platform/AGENTS.md"))
		assert.NotContains(t, readBazFile(t, root, "AGENTS.md"), "PROTO_BODY")
	})
	t.Run("above K with a shared parent uses the parent without baz_dedupe", func(t *testing.T) {
		root := run(t, siblings, "\n[rules]\nbaz_dedupe_max = 2\n")
		assert.True(t, exists(root, "svc/AGENTS.md"))
		assert.False(t, exists(root, "svc/a/AGENTS.md"))
		assert.NotContains(t, readBazFile(t, root, "AGENTS.md"), "SIB_BODY")
	})
	t.Run("baz_unscoped skip forbids the root fallback", func(t *testing.T) {
		root := run(t, threeRule, "\n[rules]\nbaz_dedupe_max = 2\nbaz_unscoped = \"skip\"\n")
		assert.True(t, exists(root, "platform/AGENTS.md"), "copies stay rather than landing in the root file")
		assert.NotContains(t, readBazFile(t, root, "AGENTS.md"), "PROTO_BODY")
	})
}
