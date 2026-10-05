package generator

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An always-on item whose targets name the root file of a preset that agents_md
// replaces is rendered into the shared AGENTS.md: the preset's own output is
// discarded, so AGENTS.md is the only place it can survive.
func TestAgentsMD_RootFileTargetsReachSharedAgentsMD(t *testing.T) {
	cases := []struct{ preset, full, base string }{
		{"claude", "CLAUDE.md", "claude.md"},
		{"gemini", "GEMINI.md", "gemini.md"},
		{"hermes", ".hermes.md", ".hermes.md"},
		{"copilot", ".github/copilot-instructions.md", "copilot-instructions.md"},
	}
	for _, tc := range cases {
		for _, target := range []string{tc.full, tc.base} {
			t.Run(tc.preset+" "+target, func(t *testing.T) {
				root := t.TempDir()
				writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig([]string{tc.preset}, "", ""))
				writeAgentsMDFile(t, root, ".ai-rulez/rules/targeted.md", "---\ntargets: ["+strconv.Quote(target)+"]\n---\nTARGETED_BODY\n")
				runAgentsMDGenerate(t, root)

				assert.Equal(t, 1, strings.Count(readAgentsMDFile(t, root, "AGENTS.md"), "TARGETED_BODY"))
			})
		}
	}
	t.Run("root file of an unconfigured preset", func(t *testing.T) {
		root := t.TempDir()
		writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig([]string{"codex"}, "", ""))
		writeAgentsMDFile(t, root, ".ai-rulez/rules/targeted.md", "---\ntargets: [\"GEMINI.md\"]\n---\nTARGETED_BODY\n")
		runAgentsMDGenerate(t, root)

		assert.NotContains(t, readAgentsMDFile(t, root, "AGENTS.md"), "TARGETED_BODY")
	})
}

// captureWarnings collects the warnings the generation emits.
func captureWarnings(t *testing.T) *[]string {
	t.Helper()
	var warned []string
	t.Cleanup(rulefiles.SetWarnSink(func(msg string, _ ...any) { warned = append(warned, msg) }))
	return &warned
}

func countContaining(msgs []string, part string) int {
	n := 0
	for _, m := range msgs {
		if strings.Contains(m, part) {
			n++
		}
	}
	return n
}

func TestAgentsMD_GeminiSettingsWhenFlagIsOff(t *testing.T) {
	cases := []struct {
		name     string
		existing string
		mcp      string
		want     []string // context.fileName after the run when it is the user's list
		wantKey  bool     // context.fileName still in the document
		rewrite  bool     // context.fileName is rewritten to [GEMINI.md, GEMINI.local.md]
		wantWarn bool
	}{
		{
			name: "value ai-rulez wrote is rewritten", existing: `{"theme":"dark","context":{"fileName":["AGENTS.md"]}}`,
			wantKey: true, rewrite: true,
		},
		{
			name: "rewritten beside other context keys", existing: `{"context":{"fileName":["AGENTS.md"],"discoveryMaxDirs":7}}`,
			wantKey: true, rewrite: true,
		},
		{
			name: "rewritten with mcp servers", existing: `{"context":{"fileName":["AGENTS.md"]}}`, mcp: agentsMDMCPServer,
			wantKey: true, rewrite: true,
		},
		{
			name: "user list lacking GEMINI.md warns", existing: `{"context":{"fileName":["CUSTOM.md","AGENTS.md"]}}`,
			wantKey: true, wantWarn: true, want: []string{"CUSTOM.md", "AGENTS.md", "GEMINI.local.md"},
		},
		{
			name: "user list with GEMINI.md is quiet", existing: `{"context":{"fileName":["GEMINI.md","AGENTS.md"]}}`,
			wantKey: true, want: []string{"GEMINI.md", "AGENTS.md", "GEMINI.local.md"},
		},
		{
			name: "unrelated list is quiet", existing: `{"context":{"fileName":["CONTEXT.md"]}}`,
			wantKey: true, want: []string{"CONTEXT.md", "GEMINI.local.md"},
		},
		{
			name:     "exactly AGENTS.md is ours even without a manifest",
			existing: `{"context":{"fileName":["AGENTS.md"]}}`, wantKey: true, rewrite: true,
		},
		{
			name: "single string is not ours", existing: `{"context":{"fileName":"AGENTS.md"}}`,
			wantKey: true, wantWarn: true, want: []string{"AGENTS.md", "GEMINI.local.md"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			warned := captureWarnings(t)
			root := t.TempDir()
			writeAgentsMDProject(t, root, agentsMDConfig([]string{"gemini"}, "", tc.mcp))
			writeAgentsMDFile(t, root, ".gemini/settings.json", tc.existing)
			runAgentsMDGenerate(t, root)

			settings := readAgentsMDFile(t, root, ".gemini/settings.json")
			assert.Equal(t, tc.wantKey, strings.Contains(settings, `"fileName"`), settings)
			switch {
			case tc.rewrite:
				assert.Equal(t, []string{"GEMINI.md", "GEMINI.local.md"}, geminiContextNames(t, root))
			case tc.wantKey:
				assert.Equal(t, tc.want, geminiContextNames(t, root), "the user's names stay, ours are appended")
			}
			assert.Equal(t, tc.wantWarn, countContaining(*warned, "GEMINI.md") > 0, *warned)
			assert.Contains(t, readAgentsMDFile(t, root, "GEMINI.md"), "ALWAYS_BODY")
			if strings.Contains(tc.existing, "theme") {
				assert.Contains(t, settings, `"theme": "dark"`)
			}
			if strings.Contains(tc.existing, "discoveryMaxDirs") {
				assert.Contains(t, settings, `"discoveryMaxDirs": 7`)
			}
			if tc.mcp != "" {
				assert.Contains(t, settings, `"mcpServers"`)
			}
		})
	}
}

func TestAgentsMD_ToggleOffRestoresGeminiContext(t *testing.T) {
	warned := captureWarnings(t)
	root := t.TempDir()
	writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig([]string{"gemini"}, "", ""))
	runAgentsMDGenerate(t, root)
	require.Contains(t, readAgentsMDFile(t, root, ".gemini/settings.json"), `"AGENTS.md"`)
	assert.NoFileExists(t, filepath.Join(root, "GEMINI.md"))

	writeAgentsMDProject(t, root, agentsMDConfig([]string{"gemini"}, "", ""))
	runAgentsMDGenerate(t, root)

	assert.Equal(t, []string{"GEMINI.md", "GEMINI.local.md"}, geminiContextNames(t, root))
	assert.Contains(t, readAgentsMDFile(t, root, "GEMINI.md"), "ALWAYS_BODY")
	assert.Empty(t, *warned)
}

func TestAgentsMD_GeminiOnlyInScopeWarns(t *testing.T) {
	cases := []struct {
		name       string
		rootPreset []string
		want       int
	}{
		{"gemini in scope only", []string{"codex"}, 1},
		{"gemini in root too", []string{"codex", "gemini"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			warned := captureWarnings(t)
			root := t.TempDir()
			scopes := "\n[[scopes]]\npath = \"packages/api\"\npresets = [\"gemini\"]\n"
			writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig(tc.rootPreset, "", scopes))
			runAgentsMDGenerate(t, root)

			assert.Equal(t, tc.want, countContaining(*warned, "gemini"), *warned)
		})
	}
}

// A skill restricted by targets is not part of the shared .agents/skills tree,
// which every reader sees; it goes through the per-preset path of the presets its
// targets allow.
func TestAgentsMD_TargetedSkillsStayPerPreset(t *testing.T) {
	cases := []struct {
		name    string
		presets []string
		targets string
		present []string
		absent  []string
	}{
		{
			name: "codex target", presets: []string{"codex", "gemini"}, targets: `["codex"]`,
			// Codex reads .agents/skills only, so its own path is that directory.
			present: []string{".agents/skills/gamma/SKILL.md", ".agents/skills/alpha/SKILL.md"},
			absent:  []string{".codex/skills/gamma/SKILL.md"},
		},
		{
			name: "gemini target", presets: []string{"codex", "gemini"}, targets: `["gemini"]`,
			present: []string{".agents/skills/gamma/SKILL.md", ".agents/skills/alpha/SKILL.md"},
			absent:  []string{".codex/skills/gamma/SKILL.md"},
		},
		{
			name: "target by path", presets: []string{"codex", "opencode"}, targets: `[".opencode/skills/"]`,
			present: []string{".opencode/skills/gamma/SKILL.md"},
			absent:  []string{".agents/skills/gamma/SKILL.md", ".codex/skills/gamma/SKILL.md"},
		},
		{
			name: "preset not configured", presets: []string{"codex"}, targets: `["gemini"]`,
			absent: []string{".agents/skills/gamma/SKILL.md", ".codex/skills/gamma/SKILL.md"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig(tc.presets, "", ""))
			writeAgentsMDFile(t, root, ".ai-rulez/skills/gamma/SKILL.md",
				"---\ndescription: Gamma skill\ntargets: "+tc.targets+"\n---\nGAMMA_BODY\n")
			runAgentsMDGenerate(t, root)

			for _, rel := range tc.present {
				assert.FileExists(t, filepath.Join(root, filepath.FromSlash(rel)))
			}
			for _, rel := range tc.absent {
				assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(rel)))
			}
		})
	}
}

// An always-on item targeted at one preset is in the shared file every AGENTS.md
// reader loads, since a shared file cannot be addressed to one reader.
func TestAgentsMD_TargetedAlwaysOnRuleReachesEveryReader(t *testing.T) {
	root := t.TempDir()
	writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig([]string{"codex", "cursor"}, "", ""))
	writeAgentsMDFile(t, root, ".ai-rulez/rules/targeted.md", "---\ntargets: [\"cursor\"]\n---\nTARGETED_BODY\n")
	runAgentsMDGenerate(t, root)

	assert.Contains(t, readAgentsMDFile(t, root, "AGENTS.md"), "TARGETED_BODY")
}

// matrixItems are the bodies of the content kinds the shared AGENTS.md and the
// rules folders divide between them, in the order of the expectation rows.
var matrixItems = []string{
	"ALWAYS_BODY", "GO_BODY", "AUTO_BODY", "MANUAL_BODY", "OVERVIEW_BODY", "SCOPEDCTX_BODY", "NEGRULE_BODY", "NEGCTX_BODY",
}

// TestAgentsMD_PlacementMatrix pins, for each kind of rule and context item, the
// exact files it lands in. Always-on items and the ones only negated globs scope
// live once in AGENTS.md; scoped, auto and manual ones keep the native files of
// the tools with a rules folder, and are inlined into AGENTS.md as well only when
// a reader of it has no folder that takes them (so folder tools see those twice).
func TestAgentsMD_PlacementMatrix(t *testing.T) {
	const agents = "AGENTS.md"
	rows := func(goRule, auto, manual, scopedCtx []string) map[string][]string {
		return map[string][]string{
			"ALWAYS_BODY": {agents}, "GO_BODY": goRule, "AUTO_BODY": auto, "MANUAL_BODY": manual,
			"OVERVIEW_BODY": {agents}, "SCOPEDCTX_BODY": scopedCtx, "NEGRULE_BODY": {agents}, "NEGCTX_BODY": {agents},
		}
	}
	everywhere := rows([]string{agents}, []string{agents}, []string{agents}, []string{agents})
	cases := []struct {
		name    string
		presets []string
		mode    string
		want    map[string][]string
	}{
		{"claude split", []string{"claude"}, "split",
			rows([]string{".claude/rules/go-style.md"}, []string{".claude/rules/auto.md"}, []string{".claude/rules/manual.md"},
				[]string{".claude/rules/context-scoped.md"})},
		{"claude inline", []string{"claude"}, "inline",
			rows([]string{".claude/rules/go-style.md"}, []string{agents}, []string{agents}, []string{".claude/rules/context-scoped.md"})},
		{"cursor split", []string{"cursor"}, "split",
			rows([]string{".cursor/rules/go-style.mdc"}, []string{".cursor/rules/auto.mdc"}, []string{".cursor/rules/manual.mdc"},
				[]string{".cursor/rules/context-scoped.mdc"})},
		{"cursor inline", []string{"cursor"}, "inline",
			rows([]string{".cursor/rules/go-style.mdc"}, []string{".cursor/rules/auto.mdc"}, []string{".cursor/rules/manual.mdc"},
				[]string{".cursor/rules/context-scoped.mdc"})},
		{"codex split", []string{"codex"}, "split", everywhere},
		{"codex inline", []string{"codex"}, "inline", everywhere},
		{"claude and codex split", []string{"claude", "codex"}, "split",
			rows([]string{".claude/rules/go-style.md", agents}, []string{".claude/rules/auto.md", agents},
				[]string{".claude/rules/manual.md", agents}, []string{".claude/rules/context-scoped.md", agents})},
		{"claude and codex inline", []string{"claude", "codex"}, "inline",
			rows([]string{".claude/rules/go-style.md", agents}, []string{agents}, []string{agents},
				[]string{".claude/rules/context-scoped.md", agents})},
		{"cursor and codex split", []string{"cursor", "codex"}, "split",
			rows([]string{".cursor/rules/go-style.mdc", agents}, []string{".cursor/rules/auto.mdc", agents},
				[]string{".cursor/rules/manual.mdc", agents}, []string{".cursor/rules/context-scoped.mdc", agents})},
		{"cursor and codex inline", []string{"cursor", "codex"}, "inline",
			rows([]string{".cursor/rules/go-style.mdc", agents}, []string{".cursor/rules/auto.mdc", agents},
				[]string{".cursor/rules/manual.mdc", agents}, []string{".cursor/rules/context-scoped.mdc", agents})},
		{"claude and cursor split", []string{"claude", "cursor"}, "split",
			rows([]string{".claude/rules/go-style.md", ".cursor/rules/go-style.mdc"},
				[]string{".claude/rules/auto.md", ".cursor/rules/auto.mdc"},
				[]string{".claude/rules/manual.md", ".cursor/rules/manual.mdc"},
				[]string{".claude/rules/context-scoped.md", ".cursor/rules/context-scoped.mdc"})},
		{"claude and cursor inline", []string{"claude", "cursor"}, "inline",
			rows([]string{".claude/rules/go-style.md", ".cursor/rules/go-style.mdc"},
				[]string{".cursor/rules/auto.mdc", agents}, []string{".cursor/rules/manual.mdc", agents},
				[]string{".claude/rules/context-scoped.md", ".cursor/rules/context-scoped.mdc"})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newAgentsMDFoldersProject(t, "agents_md = true\n", tc.presets, "\n[rules]\nmode = \""+tc.mode+"\"\n")
			writeAgentsMDFile(t, root, ".ai-rulez/rules/negrule.md", "---\nglobs: [\"!gen/**\"]\n---\nNEGRULE_BODY\n")
			writeAgentsMDFile(t, root, ".ai-rulez/context/negctx.md", "---\nglobs: [\"!gen/**\"]\n---\nNEGCTX_BODY\n")
			runAgentsMDGenerate(t, root)

			got := map[string][]string{}
			for _, rel := range agentsMDPaths(t, root) {
				if strings.HasPrefix(rel, ".agents/skills") {
					continue
				}
				text := readAgentsMDFile(t, root, rel)
				for _, body := range matrixItems {
					if n := strings.Count(text, body); n > 0 {
						assert.Equal(t, 1, n, "%s appears once in %s", body, rel)
						got[body] = append(got[body], rel)
					}
				}
			}
			for _, body := range matrixItems {
				assert.ElementsMatch(t, tc.want[body], got[body], body)
			}
		})
	}
}

// With the flag on, nothing shadows AGENTS.md for Zed, which takes the first
// match of its rules-file list.
func TestAgentsMD_NoZedShadowingFiles(t *testing.T) {
	root := t.TempDir()
	writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig(
		[]string{"claude", "codex", "cursor", "devin", "cline", "copilot", "gemini", "junie", "hermes"}, "", ""))
	runAgentsMDGenerate(t, root)

	for _, name := range []string{".rules", ".cursorrules", ".devin/rules", ".clinerules", ".github/copilot-instructions.md"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		info, err := os.Stat(path)
		if err == nil {
			assert.True(t, info.IsDir(), "%s must not exist as a file", name)
		}
	}
	assert.FileExists(t, filepath.Join(root, "AGENTS.md"))
}

func TestAgentsMD_SourceHashCoversOwnersOnlyWhenTargetsUseThem(t *testing.T) {
	hash := func(withTargets bool, presets ...string) string {
		root := t.TempDir()
		writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig(presets, "", ""))
		if withTargets {
			writeAgentsMDFile(t, root, ".ai-rulez/rules/targeted.md", "---\ntargets: [\"GEMINI.md\"]\n---\nTARGETED_BODY\n")
		}
		runAgentsMDGenerate(t, root)
		_, sourceHash := extractStoredHashes(filepath.Join(root, "AGENTS.md"))
		return sourceHash
	}
	assert.Equal(t, hash(false, "codex"), hash(false, "codex", "gemini"))
	assert.NotEqual(t, hash(true, "codex"), hash(true, "codex", "gemini"))
}

// Claude Code reads the hyphenated user-invocable key; the underscore spelling
// is ignored as an unknown field, so neither skills nor commands may emit it.
// Skills keep Claude's default (user-invocable), so no key is written for them.
func TestClaudeSkillFrontmatterUsesVendorKey(t *testing.T) {
	root := t.TempDir()
	writeAgentsMDProject(t, root, agentsMDConfig([]string{"claude"}, "", ""))
	writeAgentsMDFile(t, root, ".ai-rulez/commands/go/COMMAND.md",
		"---\ndescription: Go cmd\nuser_invocable: false\n---\nGO_BODY\n")
	writeAgentsMDFile(t, root, ".ai-rulez/skills/legacy/SKILL.md",
		"---\ndescription: Legacy\nuser_invocable: true\n---\nLEGACY_BODY\n")
	runAgentsMDGenerate(t, root)

	skill := readAgentsMDFile(t, root, ".claude/skills/alpha/SKILL.md")
	assert.NotContains(t, skill, "user-invocable")
	command := readAgentsMDFile(t, root, ".claude/skills/go/SKILL.md")
	assert.Contains(t, command, "user-invocable: true")
	// A stale authored underscore key neither leaks nor sets the hyphenated one.
	legacy := readAgentsMDFile(t, root, ".claude/skills/legacy/SKILL.md")
	assert.NotContains(t, legacy, "user-invocable")
	for _, content := range []string{skill, command, legacy} {
		assert.NotContains(t, content, "user_invocable")
	}
}
