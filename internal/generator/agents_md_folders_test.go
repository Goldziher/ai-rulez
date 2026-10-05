package generator

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAgentsMDFoldersProject extends the preset fixture (always-on rule and
// context, a glob rule, two skills, an agent, one MCP server) with an auto rule,
// a manual rule and a glob-scoped context file, the three kinds that decide what
// a rules folder and AGENTS.md each carry.
func newAgentsMDFoldersProject(t *testing.T, flag string, presets []string, extra string) string {
	t.Helper()
	root := newAgentsMDPresetProject(t, flag, presets, extra)
	writeAgentsMDFile(t, root, ".ai-rulez/rules/auto.md", "---\nactivation: auto\ndescription: when relevant\n---\nAUTO_BODY\n")
	writeAgentsMDFile(t, root, ".ai-rulez/rules/manual.md", "---\nactivation: manual\n---\nMANUAL_BODY\n")
	writeAgentsMDFile(t, root, ".ai-rulez/context/scoped.md", "---\nglobs: [\"web/**\"]\n---\nSCOPEDCTX_BODY\n")
	return root
}

func sortedPaths(groups ...[]string) []string {
	var all []string
	for _, g := range groups {
		all = append(all, g...)
	}
	sort.Strings(all)
	return all
}

func TestAgentsMD_RulesFolderPresetFileSets(t *testing.T) {
	sharedSkills := []string{
		".agents/skills/alpha/SKILL.md", ".agents/skills/alpha/references/r.md", ".agents/skills/beta/SKILL.md",
	}
	cases := []struct {
		name   string
		preset string
		want   []string
	}{
		{
			name: "cursor", preset: "cursor",
			want: sortedPaths(sharedSkills, []string{
				".cursor/agents/helper.md", ".cursor/rules/auto.mdc", ".cursor/rules/context-scoped.mdc",
				".cursor/rules/go-style.mdc", ".cursor/rules/manual.mdc", ".mcp.json", "AGENTS.md",
			}),
		},
		{
			name: "copilot", preset: "copilot",
			want: sortedPaths(sharedSkills, []string{
				".github/agents/helper.agent.md", ".github/instructions/context-scoped.instructions.md",
				".github/instructions/go-style.instructions.md", ".mcp.json", "AGENTS.md",
			}),
		},
		{
			name: "devin", preset: "devin",
			want: sortedPaths(sharedSkills, []string{
				".mcp.json", ".devin/agents/helper.md", ".devin/rules/auto.md", ".devin/rules/context-scoped.md",
				".devin/rules/go-style.md", ".devin/rules/manual.md", "AGENTS.md",
			}),
		},
		{
			name: "cline", preset: "cline",
			want: sortedPaths(sharedSkills, []string{
				".cline/agents/helper.md", ".clinerules/auto.md", ".clinerules/context-scoped.md", ".clinerules/go-style.md",
				".clinerules/manual.md", ".mcp.json", "AGENTS.md",
			}),
		},
		{
			name: "junie", preset: "junie",
			want: sortedPaths(sharedSkills, []string{
				".junie/agents/helper.md", ".junie/rules/auto.md", ".junie/rules/context-scoped.md",
				".junie/rules/go-style.md", ".junie/rules/manual.md", ".mcp.json", "AGENTS.md",
			}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newAgentsMDFoldersProject(t, "agents_md = true\n", []string{tc.preset}, "")
			runAgentsMDGenerate(t, root)

			assert.Equal(t, tc.want, agentsMDPaths(t, root))
			assert.Equal(t, tc.want, sortedPaths(filterOut(sharedManifestFiles(t, root), ".ai-rulez/")),
				"the manifest lists exactly the files written")
		})
	}
}

// filterOut drops the manifest entries below prefix (the manifest file itself).
func filterOut(paths []string, prefix string) []string {
	var kept []string
	for _, p := range paths {
		if !strings.HasPrefix(p, prefix) {
			kept = append(kept, p)
		}
	}
	return kept
}

func TestAgentsMD_RulesFolderKeepsOnlyNonAlwaysItems(t *testing.T) {
	folders := map[string]string{
		"cursor":  ".cursor/rules",
		"copilot": ".github/instructions",
		"devin":   ".devin/rules",
		"cline":   ".clinerules",
		"junie":   ".junie/rules",
	}
	for preset, dir := range folders {
		t.Run(preset, func(t *testing.T) {
			root := newAgentsMDFoldersProject(t, "agents_md = true\n", []string{preset}, "")
			runAgentsMDGenerate(t, root)

			matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(dir), "*"))
			require.NoError(t, err)
			for _, file := range matches {
				rel, _ := filepath.Rel(root, file)
				body := readAgentsMDFile(t, root, filepath.ToSlash(rel))
				assert.NotContains(t, body, "ALWAYS_BODY", "%s", rel)
				assert.NotContains(t, body, "OVERVIEW_BODY", "%s", rel)
			}
			agents := readAgentsMDFile(t, root, "AGENTS.md")
			assert.Contains(t, agents, "ALWAYS_BODY")
			assert.Contains(t, agents, "OVERVIEW_BODY")
			assert.NotContains(t, agents, "SCOPEDCTX_BODY")
			assert.NotContains(t, agents, "GO_BODY", "every AGENTS.md reader has a rules folder")
		})
	}
}

func TestAgentsMD_CopilotDropsInstructionsFileAndInlinesAutoManual(t *testing.T) {
	root := newAgentsMDFoldersProject(t, "agents_md = true\n", []string{"copilot"}, "")
	runAgentsMDGenerate(t, root)

	assert.NoFileExists(t, filepath.Join(root, ".github", "copilot-instructions.md"))
	assert.NoDirExists(t, filepath.Join(root, ".github", "skills"))
	agents := readAgentsMDFile(t, root, "AGENTS.md")
	assert.Contains(t, agents, "AUTO_BODY")
	assert.Contains(t, agents, "_When relevant: when relevant_")
	assert.Contains(t, agents, "MANUAL_BODY")
	assert.NotContains(t, agents, "GO_BODY")
	assert.Contains(t, readAgentsMDFile(t, root, ".github/instructions/go-style.instructions.md"), "applyTo: '**/*.go'")
	assert.NoFileExists(t, filepath.Join(root, ".github", "instructions", "auto.instructions.md"))
}

func TestAgentsMD_JunieDropsGuidelinesAndOwnSkills(t *testing.T) {
	root := newAgentsMDFoldersProject(t, "agents_md = true\n", []string{"junie"}, "")
	runAgentsMDGenerate(t, root)

	assert.NoFileExists(t, filepath.Join(root, ".junie", "guidelines.md"))
	assert.NoDirExists(t, filepath.Join(root, ".junie", "skills"))
	assert.FileExists(t, filepath.Join(root, ".agents", "skills", "alpha", "SKILL.md"))
}

func TestAgentsMD_JunieInlineModeKeepsEveryRuleInAgentsMD(t *testing.T) {
	root := newAgentsMDFoldersProject(t, "agents_md = true\n", []string{"junie"}, "\n[rules]\nmode = \"inline\"\n")
	runAgentsMDGenerate(t, root)

	assert.NoFileExists(t, filepath.Join(root, ".junie", "guidelines.md"))
	agents := readAgentsMDFile(t, root, "AGENTS.md")
	for _, body := range []string{"ALWAYS_BODY", "GO_BODY", "AUTO_BODY", "MANUAL_BODY", "OVERVIEW_BODY", "SCOPEDCTX_BODY"} {
		assert.Contains(t, agents, body, "inline mode leaves junie no rules folder to hold %s", body)
	}
}

func TestAgentsMD_ScopedInlineDecision(t *testing.T) {
	cases := []struct {
		name       string
		presets    []string
		extra      string
		scoped     bool // glob rules (with Applies to) and glob context in AGENTS.md
		autoManual bool // auto and manual rules in AGENTS.md
	}{
		{name: "claude and cursor", presets: []string{"claude", "cursor"}},
		{name: "every folder preset", presets: []string{"claude", "cursor", "devin", "cline", "junie", "antigravity"}},
		{name: "copilot", presets: []string{"copilot"}, autoManual: true},
		{name: "claude and codex", presets: []string{"claude", "codex"}, scoped: true, autoManual: true},
		{name: "cursor and gemini", presets: []string{"cursor", "gemini"}, scoped: true, autoManual: true},
		{name: "cursor and amp", presets: []string{"cursor", "amp"}, scoped: true, autoManual: true},
		{name: "cursor and hermes", presets: []string{"cursor", "hermes"}, scoped: true, autoManual: true},
		{
			name: "claude in inline mode", presets: []string{"claude"}, extra: "\n[rules]\nmode = \"inline\"\n",
			autoManual: true,
		},
		{
			name: "antigravity in inline mode", presets: []string{"antigravity"}, extra: "\n[rules]\nmode = \"inline\"\n",
			autoManual: true,
		},
		{
			name: "junie in inline mode", presets: []string{"junie"}, extra: "\n[rules]\nmode = \"inline\"\n",
			scoped: true, autoManual: true,
		},
		{
			name: "cursor ignores the inline mode", presets: []string{"cursor"}, extra: "\n[rules]\nmode = \"inline\"\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newAgentsMDFoldersProject(t, "agents_md = true\n", tc.presets, tc.extra)
			runAgentsMDGenerate(t, root)
			agents := readAgentsMDFile(t, root, "AGENTS.md")

			assert.Contains(t, agents, "ALWAYS_BODY")
			assert.Contains(t, agents, "OVERVIEW_BODY")
			assert.Equal(t, tc.scoped, containsAll(agents, "GO_BODY", "_Applies to: `**/*.go`_", "SCOPEDCTX_BODY"), agents)
			assert.Equal(t, tc.autoManual, containsAll(agents, "AUTO_BODY", "_When relevant: when relevant_", "MANUAL_BODY"), agents)
		})
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

func TestAgentsMD_ToggleRestoresDroppedFiles(t *testing.T) {
	for _, preset := range []string{"cursor", "copilot", "devin", "cline", "junie"} {
		t.Run(preset, func(t *testing.T) {
			root := newAgentsMDFoldersProject(t, "", []string{preset}, "")
			runAgentsMDGenerate(t, root)
			offTree := agentsMDSnapshot(t, root)

			setAgentsMDFlag(t, root, "agents_md = true\n", []string{preset})
			runAgentsMDGenerate(t, root)
			onTree := agentsMDSnapshot(t, root)
			var dropped []string
			for rel := range offTree {
				if _, ok := onTree[rel]; !ok {
					dropped = append(dropped, rel)
				}
			}
			assert.NotEmpty(t, dropped, "the flag drops files of %s", preset)

			setAgentsMDFlag(t, root, "", []string{preset})
			runAgentsMDGenerate(t, root)
			assert.Equal(t, offTree, agentsMDSnapshot(t, root), "off, on, off ends where off began")
			for _, rel := range dropped {
				assert.FileExists(t, filepath.Join(root, filepath.FromSlash(rel)), "toggling off restores %s", rel)
			}
		})
	}
}

func TestAgentsMD_FoldersSecondRunIsIdempotent(t *testing.T) {
	root := newAgentsMDFoldersProject(t, "agents_md = true\n",
		[]string{"cursor", "copilot", "devin", "cline", "junie", "claude"}, "")
	runAgentsMDGenerate(t, root)
	first := agentsMDSnapshot(t, root)
	runAgentsMDGenerate(t, root)
	assert.Equal(t, first, agentsMDSnapshot(t, root))
}

func TestAgentsMD_SameDecisionSameAgentsMD(t *testing.T) {
	groups := map[string][][]string{
		"folders only": {
			{"claude", "cursor"}, {"cursor", "devin"}, {"cline", "junie", "claude"},
		},
		"auto and manual only": {
			{"copilot"}, {"copilot", "cursor"}, {"copilot", "claude", "devin"},
		},
		"scoped": {
			{"claude", "codex"}, {"cursor", "gemini"}, {"copilot", "opencode"}, {"devin", "amp"},
		},
	}
	for name, combos := range groups {
		t.Run(name, func(t *testing.T) {
			var want string
			for i, presets := range combos {
				root := newAgentsMDFoldersProject(t, "agents_md = true\n", presets, "")
				runAgentsMDGenerate(t, root)
				got := readAgentsMDFile(t, root, "AGENTS.md")
				if i == 0 {
					want = got
					continue
				}
				assert.Equal(t, want, got, "%v", presets)
			}
		})
	}
}

func TestAgentsMD_DecisionChangesSharedHash(t *testing.T) {
	hash := func(presets ...string) string {
		root := newAgentsMDFoldersProject(t, "agents_md = true\n", presets, "")
		runAgentsMDGenerate(t, root)
		agents := readAgentsMDFile(t, root, "AGENTS.md")
		for _, line := range strings.Split(agents, "\n") {
			if strings.HasPrefix(line, "Source-Hash:") {
				return line
			}
		}
		t.Fatal("no Source-Hash line")
		return ""
	}
	assert.Equal(t, hash("claude", "cursor"), hash("devin"), "same decision, same provenance")
	assert.NotEqual(t, hash("claude", "cursor"), hash("claude", "codex"))
	assert.NotEqual(t, hash("claude", "cursor"), hash("copilot"))
}

func TestAgentsMD_CopilotLocalRulesStayFiles(t *testing.T) {
	root := newAgentsMDFoldersProject(t, "agents_md = true\n", []string{"copilot"}, "")
	writeAgentsMDFile(t, root, ".ai-rulez/local/rules/mine.md", "# Mine\n\nLOCAL_BODY\n")
	runAgentsMDGenerate(t, root)

	assert.Contains(t, readAgentsMDFile(t, root, ".github/instructions/mine.local.instructions.md"), "LOCAL_BODY")
	assert.NotContains(t, readAgentsMDFile(t, root, "AGENTS.md"), "LOCAL_BODY")
}

// setAgentsMDFlag rewrites only config.toml, leaving the generated manifest in
// .ai-rulez so the next run can clean up what the flag change made stale.
func setAgentsMDFlag(t *testing.T, root, flag string, presets []string) {
	t.Helper()
	writeAgentsMDFile(t, root, ".ai-rulez/config.toml", flag+agentsMDConfig(presets, "", agentsMDMCPServer))
}
