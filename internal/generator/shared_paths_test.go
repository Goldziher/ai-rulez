package generator

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const sharedPathsMCP = `
[[mcp_servers]]
name = "local"
command = "uvx"
args = ["a"]
description = "Local server"

[[mcp_servers]]
name = "remote"
transport = "http"
url = "https://example.com/mcp"
description = "Remote server"
`

// sharedPathsTypedSkill sets every skill key some preset writes to the shared
// .agents/skills tree, with values that need their YAML type kept (a list, a
// bool, a nested map, a date), so each pair of writers is held to one rendering.
const sharedPathsTypedSkill = `---
name: typed-skill
description: Use when checking shared skill frontmatter.
short-description: Short
license: MIT
compatibility: claude, codex
allowed-tools: Read Grep
disable-model-invocation: true
user-invocable: true
paths:
  - "src/**/*.py"
metadata:
  owner: team-a
  reviewed: {by: alice, date: 2026-10-01}
  tags: [a, b]
---
TYPED_SKILL_BODY
`

// sharedPathsTypedAgent carries typed invocation switches, which .github/agents
// (copilot and copilot-cli) must render the same way.
const sharedPathsTypedAgent = `---
description: Typed agent
kind: local
temperature: 0.2
max_turns: 5
timeout_mins: 10
user-invocable: true
disable-model-invocation: false
tools: [read, grep]
---
TYPED_AGENT_BODY
`

// sharedPathsVariant is one way of configuring the run.
type sharedPathsVariant struct {
	name     string
	agentsMD bool
	rules    string // rules mode, "" for the default (split)
}

// sharedPathsVariants lists the configurations in which every shared path must
// agree byte for byte. With the default split rules mode and agents_md off, a
// preset with a rules folder legitimately writes an AGENTS.md without the rules
// that one without a folder inlines; that case is resolved, not equal, and is
// covered by TestSharedPaths_SplitRulesYieldToInliningWriter.
var sharedPathsVariants = []sharedPathsVariant{
	{"agents_md off, inline rules", false, "inline"},
	{"agents_md on, split rules", true, ""},
	{"agents_md on, inline rules", true, "inline"},
}

func (v sharedPathsVariant) extra() (flag, tables string) {
	if v.agentsMD {
		flag = "agents_md = true\n"
	}
	tables = sharedPathsMCP
	if v.rules != "" {
		tables += "\n[rules]\nmode = \"" + v.rules + "\"\n"
	}
	return flag, tables
}

// newSharedPathsProject writes a project exercising every content kind (rules of
// every activation, scoped context, a command, skills with resources, an agent
// with the delegation builtin, MCP servers) for the given presets.
func newSharedPathsProject(t *testing.T, v sharedPathsVariant, presets []string) string {
	t.Helper()
	root := t.TempDir()
	flag, tables := v.extra()
	writeAgentsMDProject(t, root, agentsMDConfig(presets, flag+"builtins = [\"agent-delegation\"]\n", tables))
	writeAgentsMDFile(t, root, ".ai-rulez/rules/go-style.md", agentsMDGoStyleRule)
	writeAgentsMDFile(t, root, ".ai-rulez/rules/auto-rule.md", "---\nactivation: auto\ndescription: when X\n---\nAUTO_BODY\n")
	writeAgentsMDFile(t, root, ".ai-rulez/rules/manual-rule.md", "---\nactivation: manual\n---\nMANUAL_BODY\n")
	writeAgentsMDFile(t, root, ".ai-rulez/context/go-ctx.md", "---\nglobs: [\"internal/**\"]\n---\nGLOB_CONTEXT\n")
	writeAgentsMDFile(t, root, ".ai-rulez/commands/review.md", "---\ndescription: Review code\n---\nREVIEW_BODY\n")
	writeAgentsMDFile(t, root, ".ai-rulez/skills/typed-skill/SKILL.md", sharedPathsTypedSkill)
	writeAgentsMDFile(t, root, ".ai-rulez/agents/typed-agent.md", sharedPathsTypedAgent)
	return root
}

// loadSharedPathsConfig loads the variant's project once; renderSharedPaths then
// swaps the presets on a copy, which is far cheaper than a project per pair.
func loadSharedPathsConfig(t *testing.T, v sharedPathsVariant) *config.Config {
	t.Helper()
	root := newSharedPathsProject(t, v, []string{"codex"})
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	return cfg
}

// renderSharedPaths renders the project in memory with the given presets. A
// conflict between presets is returned as the error; otherwise the result maps
// every written path to the presets that write it, with the merged outputs.
func renderSharedPaths(t *testing.T, base *config.Config, presets []string) (map[string][]string, []config.OutputFile, error) {
	t.Helper()
	cfg := *base
	// The generator resolves placeholders in the servers in place, so each run
	// gets its own copies.
	cfg.MCPServers = make(map[string]*config.MCPServer, len(base.MCPServers))
	for name, server := range base.MCPServers {
		clone := *server
		clone.Args = slices.Clone(server.Args)
		cfg.MCPServers[name] = &clone
	}
	cfg.Presets = make([]config.Preset, len(presets))
	for i, name := range presets {
		cfg.Presets[i] = config.Preset{BuiltIn: name}
	}
	render, err := NewGenerator(&cfg).renderPresets("")
	require.NoError(t, err)
	flat, err := flattenPresetOutputs(nil, render.byPreset)
	if err != nil {
		return nil, nil, err
	}
	writers := map[string][]string{}
	for name, outputs := range render.byPreset {
		for _, o := range outputs {
			if o.IsDir {
				continue
			}
			rel, relErr := filepath.Rel(cfg.BaseDir, o.Path)
			require.NoError(t, relErr)
			writers[filepath.ToSlash(rel)] = append(writers[filepath.ToSlash(rel)], name)
		}
	}
	return writers, flat, nil
}

func allBuiltinPresets() []string {
	return config.IndividualPresetNames()
}

// TestSharedPaths_AllPresetsAgree enables every built-in preset at once: each
// path several presets write must carry identical bytes, otherwise generation
// fails with a conflict naming the presets.
func TestSharedPaths_AllPresetsAgree(t *testing.T) {
	for _, v := range sharedPathsVariants {
		t.Run(v.name, func(t *testing.T) {
			// Arrange / Act
			writers, _, err := renderSharedPaths(t, loadSharedPathsConfig(t, v), allBuiltinPresets())

			// Assert
			require.NoError(t, err)
			if v.agentsMD {
				assert.Equal(t, []string{sharedOutputsKey}, writers["AGENTS.md"])
			} else {
				const minAgentsMDWriters = 20 // every preset with a root AGENTS.md
				assert.GreaterOrEqual(t, len(writers["AGENTS.md"]), minAgentsMDWriters)
			}
		})
	}
}

// TestSharedPaths_PairsAgree renders every preset alone, then every pair of
// presets that write a common path, and requires the pair to generate without a
// conflict.
func TestSharedPaths_PairsAgree(t *testing.T) {
	for _, v := range sharedPathsVariants {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			base := loadSharedPathsConfig(t, v)
			names := allBuiltinPresets()
			alone := make(map[string]map[string][]string, len(names))
			for _, name := range names {
				writers, _, err := renderSharedPaths(t, base, []string{name})
				require.NoError(t, err, name)
				alone[name] = writers
			}

			// Act / Assert
			for i, a := range names {
				for _, b := range names[i+1:] {
					if !sharePath(alone[a], alone[b]) {
						continue
					}
					t.Run(a+"+"+b, func(t *testing.T) {
						t.Parallel()
						_, _, err := renderSharedPaths(t, base, []string{a, b})
						assert.NoError(t, err)
					})
				}
			}
		})
	}
}

// TestSharedPaths_SplitRulesYieldToInliningWriter covers the one divergence that
// is genuine: with agents_md off and the default split rules mode, a preset with
// a rules folder keeps the rules in that folder and its AGENTS.md omits them,
// while a preset without a folder inlines them. The complete file wins whatever
// the preset names sort like (dropping it would take the rules from the tools
// without a folder); presets with folders agree among themselves.
func TestSharedPaths_SplitRulesYieldToInliningWriter(t *testing.T) {
	base := loadSharedPathsConfig(t, sharedPathsVariant{name: "agents_md off, split rules"})
	folders := []string{"junie", "kiro", "augment", "kilo", "bob", "grok", "codewhale", "omp", "qoder"}

	agentsMD := func(flat []config.OutputFile) string {
		for _, o := range flat {
			if filepath.Base(o.Path) == "AGENTS.md" {
				return o.Content
			}
		}
		return ""
	}

	for _, folder := range folders {
		// amp sorts before and xum after every folder preset, codex among them.
		for _, inliner := range []string{"amp", "codex", "xum"} {
			t.Run(folder+"+"+inliner, func(t *testing.T) {
				// Act
				_, flat, err := renderSharedPaths(t, base, []string{folder, inliner})

				// Assert
				require.NoError(t, err)
				assert.Contains(t, agentsMD(flat), "GO_BODY", "the file with every rule inlined must win")
			})
		}
	}

	t.Run("folder presets agree", func(t *testing.T) {
		_, flat, err := renderSharedPaths(t, base, folders)
		require.NoError(t, err)
		assert.NotContains(t, agentsMD(flat), "GO_BODY", "rules stay in the folders")
	})
}

func sharePath(a, b map[string][]string) bool {
	for path := range a {
		if _, ok := b[path]; ok {
			return true
		}
	}
	return false
}

// TestFlattenPresetOutputs_ConflictNamesPresets pins the guard itself.
func TestFlattenPresetOutputs_ConflictNamesPresets(t *testing.T) {
	tests := []struct {
		name    string
		outputs map[string][]config.OutputFile
		wantErr []string
		want    string // content kept for the single output, when set
	}{
		{
			name: "identical content from two presets is kept once",
			outputs: map[string][]config.OutputFile{
				"a": {{Path: "AGENTS.md", Content: "same"}},
				"b": {{Path: "AGENTS.md", Content: "same"}},
			},
		},
		{
			name: "different content names both presets",
			outputs: map[string][]config.OutputFile{
				"a": {{Path: "AGENTS.md", Content: "one"}},
				"b": {{Path: "AGENTS.md", Content: "two"}},
			},
			wantErr: []string{"AGENTS.md", "b differs from a"},
		},
		{
			name: "different raw content conflicts",
			outputs: map[string][]config.OutputFile{
				"a": {{Path: "x.bin", RawContent: []byte{1}}},
				"b": {{Path: "x.bin", RawContent: []byte{2}}},
			},
			wantErr: []string{"x.bin"},
		},
		{
			name: "a root file omitting rules yields to the complete one, whichever sorts first",
			outputs: map[string][]config.OutputFile{
				"a": {{Path: "AGENTS.md", Content: "no rules", OmitsRules: true}},
				"b": {{Path: "AGENTS.md", Content: "all rules"}},
			},
			want: "all rules",
		},
		{
			name: "a root file omitting rules yields to the complete one that sorts first",
			outputs: map[string][]config.OutputFile{
				"a": {{Path: "AGENTS.md", Content: "all rules"}},
				"b": {{Path: "AGENTS.md", Content: "no rules", OmitsRules: true}},
			},
			want: "all rules",
		},
		{
			name: "two root files omitting different rules conflict",
			outputs: map[string][]config.OutputFile{
				"a": {{Path: "AGENTS.md", Content: "x", OmitsRules: true}},
				"b": {{Path: "AGENTS.md", Content: "y", OmitsRules: true}},
			},
			wantErr: []string{"AGENTS.md", "b differs from a"},
		},
		{
			name: "shared directories are not conflicts",
			outputs: map[string][]config.OutputFile{
				"a": {{Path: ".agents", IsDir: true}},
				"b": {{Path: ".agents", IsDir: true}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			flat, err := flattenPresetOutputs(nil, tt.outputs)

			// Assert
			if len(tt.wantErr) == 0 {
				require.NoError(t, err)
				require.Len(t, flat, 1)
				if tt.want != "" {
					assert.Equal(t, tt.want, flat[0].Content)
				}
				return
			}
			require.Error(t, err)
			for _, want := range tt.wantErr {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

// TestSharedPaths_ScopedRuleFilesMatch checks that two presets of one tool lay a
// scope's rule files out the same way: enabled together they must not leave two
// copies of one rule under different names (Copilot would load both).
func TestSharedPaths_ScopedRuleFilesMatch(t *testing.T) {
	tests := []struct {
		name string
		a, b string
	}{
		{"copilot and copilot-cli", "copilot", "copilot-cli"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			scopeBlock := "\n[profiles]\napi = [\"api\"]\n\n[[scopes]]\npath = \"packages/api\"\nprofile = \"api\"\npresets = [\"" +
				tt.a + "\", \"" + tt.b + "\"]\n"
			root := t.TempDir()
			writeAgentsMDProject(t, root, agentsMDConfig([]string{tt.a, tt.b}, "", agentsMDMCPServer+scopeBlock))
			writeAgentsMDFile(t, root, ".ai-rulez/domains/api/rules/api-style.md", "---\nglobs: [\"**/*.go\"]\n---\nAPI_STYLE_BODY\n")
			runAgentsMDGenerate(t, root)

			// Act
			var ruleFiles []string
			for _, rel := range agentsMDPaths(t, root) {
				if strings.Contains(rel, "api-style") {
					ruleFiles = append(ruleFiles, rel)
				}
			}

			// Assert
			assert.Len(t, ruleFiles, 1, "one copy of the scoped rule: %v", ruleFiles)
		})
	}
}

// TestSharedPaths_SkillTreeIsOneRendering checks the canonical form directly:
// whichever writers are enabled, a shared skill file is the same bytes and
// carries every key with its type.
func TestSharedPaths_SkillTreeIsOneRendering(t *testing.T) {
	// Arrange
	base := loadSharedPathsConfig(t, sharedPathsVariants[0])
	content := func(rel string, presets []string) string {
		_, flat, err := renderSharedPaths(t, base, presets)
		require.NoError(t, err)
		for _, o := range flat {
			if filepath.ToSlash(o.Path) == filepath.ToSlash(filepath.Join(base.BaseDir, rel)) {
				return o.Content
			}
		}
		return ""
	}
	const agentsSkill = ".agents/skills/typed-skill/SKILL.md"
	want := content(agentsSkill, []string{"codex"})

	// Assert
	assert.Contains(t, want, "name: typed-skill\ndescription: \"Use when checking shared skill frontmatter.\"\n")
	for _, key := range []string{"disable-model-invocation: true", "user-invocable: true", "paths:", "date: 2026-10-01", "short-description:"} {
		assert.Contains(t, want, key)
	}
	for _, name := range []string{"cursor", "gemini", "antigravity", "amp", "pi", "aiassistant", "goose", "zed", "muse", "letta", "replit"} {
		assert.Equal(t, want, content(agentsSkill, []string{name}), name)
	}
	const githubSkill = ".github/skills/typed-skill/SKILL.md"
	assert.Equal(t, want, content(githubSkill, []string{"copilot"}), "copilot")
	assert.Equal(t, want, content(githubSkill, []string{"copilot-cli"}), "copilot-cli")
}
