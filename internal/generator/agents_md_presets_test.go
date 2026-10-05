package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	agentsMDGoStyleRule = "---\nglobs: [\"**/*.go\"]\n---\nGO_BODY\n"
	agentsMDMCPServer   = "\n[[mcp_servers]]\nname = \"x\"\ncommand = \"uvx\"\nargs = [\"a\"]\n"
)

func writeAgentsMDFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func readAgentsMDFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	require.NoError(t, err)
	return string(data)
}

// afterBanner is the part of a generated file following its comment banner.
func afterBanner(content string) string {
	return strings.TrimSpace(content[strings.Index(content, "-->")+len("-->"):])
}

func agentsMDPaths(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	for rel := range agentsMDSnapshot(t, root) {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	return paths
}

// newAgentsMDPresetProject writes the shared fixture plus a path-scoped rule and
// one MCP server, so rule files, settings documents and skills are all in play.
func newAgentsMDPresetProject(t *testing.T, flag string, presets []string, extra string) string {
	t.Helper()
	root := t.TempDir()
	writeAgentsMDProject(t, root, flag+agentsMDConfig(presets, "", agentsMDMCPServer+extra))
	writeAgentsMDFile(t, root, ".ai-rulez/rules/go-style.md", agentsMDGoStyleRule)
	return root
}

func TestAgentsMD_PresetFileSets(t *testing.T) {
	skills := []string{
		".agents/skills/alpha/SKILL.md", ".agents/skills/alpha/references/r.md", ".agents/skills/beta/SKILL.md",
	}
	join := func(groups ...[]string) []string {
		var all []string
		for _, g := range groups {
			all = append(all, g...)
		}
		sort.Strings(all)
		return all
	}
	cases := []struct {
		name    string
		presets []string
		want    []string
	}{
		{
			name:    "claude",
			presets: []string{"claude"},
			want: []string{
				".claude/agents/helper.md", ".claude/rules/go-style.md", ".claude/settings.json",
				".claude/skills/alpha/SKILL.md", ".claude/skills/alpha/references/r.md", ".claude/skills/beta/SKILL.md",
				".mcp.json", "AGENTS.md", "CLAUDE.md",
			},
		},
		{
			name:    "gemini",
			presets: []string{"gemini"},
			want:    join(skills, []string{".gemini/agents/helper.md", ".gemini/settings.json", ".mcp.json", "AGENTS.md"}),
		},
		{
			name:    "antigravity",
			presets: []string{"antigravity"},
			want: join(skills, []string{
				".agents/agents/helper.md", ".agents/rules/go-style.md", ".agents/mcp_config.json", ".agents/settings.json", ".mcp.json", "AGENTS.md",
			}),
		},
		{
			name:    "hermes",
			presets: []string{"hermes"},
			want:    join(skills, []string{".mcp.json", "AGENTS.md"}),
		},
		{
			name:    "gemini and antigravity",
			presets: []string{"gemini", "antigravity"},
			want: join(skills, []string{
				".agents/agents/helper.md", ".agents/rules/go-style.md", ".agents/mcp_config.json", ".agents/settings.json", ".gemini/agents/helper.md",
				".gemini/settings.json", ".mcp.json", "AGENTS.md",
			}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newAgentsMDPresetProject(t, "agents_md = true\n", tc.presets, "")
			runAgentsMDGenerate(t, root)

			assert.Equal(t, tc.want, agentsMDPaths(t, root))
			assert.NoFileExists(t, filepath.Join(root, "GEMINI.md"))
			assert.NoFileExists(t, filepath.Join(root, ".hermes.md"))
		})
	}
}

func TestAgentsMD_ClaudeShim(t *testing.T) {
	root := newAgentsMDPresetProject(t, "agents_md = true\n", []string{"claude"}, "")
	runAgentsMDGenerate(t, root)

	claude := readAgentsMDFile(t, root, "CLAUDE.md")
	assert.Contains(t, claude, "GENERATED FILE")
	assert.Equal(t, "@AGENTS.md", afterBanner(claude))
	for _, body := range []string{"ALWAYS_BODY", "OVERVIEW_BODY", "GO_BODY"} {
		assert.NotContains(t, claude, body, "the shim must not duplicate AGENTS.md")
	}
	agents := readAgentsMDFile(t, root, "AGENTS.md")
	assert.Contains(t, agents, "ALWAYS_BODY")
	assert.Contains(t, agents, "OVERVIEW_BODY")

	rule := readAgentsMDFile(t, root, ".claude/rules/go-style.md")
	assert.Contains(t, rule, "GO_BODY")
	assert.NoFileExists(t, filepath.Join(root, ".claude", "rules", "always.md"))
	assert.NoDirExists(t, filepath.Join(root, ".agents", "skills", "claude"))
}

func TestAgentsMD_ClaudeSplitModeKeepsOnlyNonAlwaysRules(t *testing.T) {
	root := newAgentsMDPresetProject(t, "agents_md = true\n", []string{"claude"}, "\n[rules]\nmode = \"split\"\n")
	writeAgentsMDFile(t, root, ".ai-rulez/rules/manual.md", "---\nactivation: manual\n---\nMANUAL_BODY\n")
	runAgentsMDGenerate(t, root)

	assert.FileExists(t, filepath.Join(root, ".claude", "rules", "go-style.md"))
	assert.FileExists(t, filepath.Join(root, ".claude", "rules", "manual.md"))
	assert.NoFileExists(t, filepath.Join(root, ".claude", "rules", "always.md"))
	assert.NotContains(t, readAgentsMDFile(t, root, "CLAUDE.md"), "ALWAYS_BODY")
}

func TestAgentsMD_GeminiSettingsMerge(t *testing.T) {
	cases := []struct {
		name     string
		existing string
		want     []string
		keys     map[string]any
	}{
		{name: "no file", existing: "", want: []string{"AGENTS.md", "GEMINI.local.md"}},
		{
			name:     "user keys and names survive",
			existing: `{"theme":"dark","context":{"fileName":["CUSTOM.md"],"discoveryMaxDirs":7}}`,
			want:     []string{"CUSTOM.md", "AGENTS.md", "GEMINI.local.md"},
			keys:     map[string]any{"theme": "dark"},
		},
		{
			name:     "single string name",
			existing: `{"context":{"fileName":"CUSTOM.md"}}`,
			want:     []string{"CUSTOM.md", "AGENTS.md", "GEMINI.local.md"},
		},
		{
			name:     "already listed stays single",
			existing: `{"context":{"fileName":["AGENTS.md","CUSTOM.md"]}}`,
			want:     []string{"AGENTS.md", "CUSTOM.md", "GEMINI.local.md"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig([]string{"gemini"}, "", ""))
			if tc.existing != "" {
				writeAgentsMDFile(t, root, ".gemini/settings.json", tc.existing)
			}

			runAgentsMDGenerate(t, root)
			first := readAgentsMDFile(t, root, ".gemini/settings.json")
			var doc struct {
				Context struct {
					FileName []string `json:"fileName"`
				} `json:"context"`
			}
			require.NoError(t, json.Unmarshal([]byte(first), &doc))
			assert.Equal(t, tc.want, doc.Context.FileName)
			assert.NotContains(t, first, `"mcpServers"`, "no [[mcp_servers]]: the user's servers are not ours to touch")
			for key, value := range tc.keys {
				var all map[string]any
				require.NoError(t, json.Unmarshal([]byte(first), &all))
				assert.Equal(t, value, all[key])
			}
			if strings.Contains(tc.existing, "discoveryMaxDirs") {
				assert.Contains(t, first, `"discoveryMaxDirs": 7`)
			}

			runAgentsMDGenerate(t, root)
			assert.Equal(t, first, readAgentsMDFile(t, root, ".gemini/settings.json"), "second run is idempotent")
		})
	}
}

func TestAgentsMD_AntigravityWithGeminiUsesRulesFolder(t *testing.T) {
	cases := []struct {
		name, flag string
		wantRule   bool
		wantGemini bool
	}{
		{"flag on", "agents_md = true\n", true, false},
		// Without the flag both write GEMINI.md, so antigravity keeps everything inline.
		{"flag off", "", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newAgentsMDPresetProject(t, tc.flag, []string{"gemini", "antigravity"}, "")
			runAgentsMDGenerate(t, root)

			assert.Equal(t, tc.wantRule, fileExists(filepath.Join(root, ".agents", "rules", "go-style.md")))
			assert.Equal(t, tc.wantGemini, fileExists(filepath.Join(root, "GEMINI.md")))
		})
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestAgentsMD_TargetsOfReadingPresetsReachSharedAgentsMD(t *testing.T) {
	cases := []struct {
		name    string
		presets []string
		target  string
		want    bool
	}{
		{"claude target, claude configured", []string{"claude"}, "claude", true},
		{"gemini target, gemini configured", []string{"gemini"}, "gemini", true},
		{"antigravity target, antigravity configured", []string{"antigravity"}, "antigravity", true},
		{"hermes target, hermes configured", []string{"hermes"}, "hermes", true},
		{"codex target, codex configured", []string{"codex"}, "codex", true},
		{"cursor target, never in AGENTS.md", []string{"claude"}, "cursor", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig(tc.presets, "", ""))
			writeAgentsMDFile(t, root, ".ai-rulez/rules/targeted.md", "---\ntargets: ["+`"`+tc.target+`"`+"]\n---\nTARGETED_BODY\n")
			runAgentsMDGenerate(t, root)

			assert.Equal(t, tc.want, strings.Contains(readAgentsMDFile(t, root, "AGENTS.md"), "TARGETED_BODY"))
		})
	}
}

func TestAgentsMD_ToggleRestoresRootFiles(t *testing.T) {
	cases := []struct {
		name    string
		presets []string
		root    string // root file that exists only with the flag off
	}{
		{"claude", []string{"claude"}, "CLAUDE.md"},
		{"gemini", []string{"gemini"}, "GEMINI.md"},
		{"antigravity", []string{"antigravity"}, "GEMINI.md"},
		{"hermes", []string{"hermes"}, ".hermes.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newAgentsMDPresetProject(t, "", tc.presets, "")
			runAgentsMDGenerate(t, root)
			offTree := agentsMDSnapshot(t, root)
			require.FileExists(t, filepath.Join(root, tc.root))
			offRoot := readAgentsMDFile(t, root, tc.root)

			writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig(tc.presets, "", agentsMDMCPServer))
			runAgentsMDGenerate(t, root)
			if tc.root == "CLAUDE.md" {
				assert.Equal(t, "@AGENTS.md", afterBanner(readAgentsMDFile(t, root, tc.root)))
			} else {
				assert.NoFileExists(t, filepath.Join(root, tc.root), "flag on removes %s", tc.root)
			}
			assert.NotContains(t, sharedManifestFiles(t, root), "GEMINI.md")

			writeAgentsMDProject(t, root, agentsMDConfig(tc.presets, "", agentsMDMCPServer))
			runAgentsMDGenerate(t, root)
			assert.Equal(t, offRoot, readAgentsMDFile(t, root, tc.root), "flag off restores %s", tc.root)
			if tc.name != "gemini" {
				// Gemini's settings.json keeps the context.fileName entry: it is a
				// merged document and the user may rely on it.
				assert.Equal(t, offTree, agentsMDSnapshot(t, root), "off, on, off ends where off began")
			}
		})
	}
}

func TestAgentsMD_ScopeClaudeShimImportsNestedAgentsMD(t *testing.T) {
	root := t.TempDir()
	writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig([]string{"claude"}, "", `
[profiles]
api = ["api"]

[[scopes]]
path = "packages/api"
profile = "api"
presets = ["claude"]
`))
	writeAgentsMDFile(t, root, ".ai-rulez/domains/api/rules/api-style.md", "# Api Style\n\nAPI_STYLE_BODY\n")
	runAgentsMDGenerate(t, root)

	nested := readAgentsMDFile(t, root, "packages/api/CLAUDE.md")
	assert.Equal(t, "@AGENTS.md", afterBanner(nested))
	assert.Contains(t, readAgentsMDFile(t, root, "packages/api/AGENTS.md"), "API_STYLE_BODY")
	assert.Equal(t, "@AGENTS.md", afterBanner(readAgentsMDFile(t, root, "CLAUDE.md")))
}

func TestAgentsMD_ScopeGeminiLeavesContextToRootSettings(t *testing.T) {
	root := t.TempDir()
	writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig([]string{"gemini"}, "", `
[profiles]
api = ["api"]

[[scopes]]
path = "packages/api"
profile = "api"
presets = ["gemini"]
`))
	writeAgentsMDFile(t, root, ".ai-rulez/domains/api/rules/api-style.md", "# Api Style\n\nAPI_STYLE_BODY\n")
	runAgentsMDGenerate(t, root)

	assert.FileExists(t, filepath.Join(root, ".gemini", "settings.json"))
	assert.NoFileExists(t, filepath.Join(root, "packages", "api", ".gemini", "settings.json"))
	assert.NoFileExists(t, filepath.Join(root, "packages", "api", "GEMINI.md"))
	assert.FileExists(t, filepath.Join(root, "packages", "api", "AGENTS.md"))
}

func TestAgentsMD_LocalClaudeContentKeepsLocalFile(t *testing.T) {
	root := newAgentsMDPresetProject(t, "agents_md = true\n", []string{"claude"}, "")
	writeAgentsMDFile(t, root, ".ai-rulez/local/rules/mine.md", "# Mine\n\nLOCAL_BODY\n")
	runAgentsMDGenerate(t, root)

	assert.Contains(t, readAgentsMDFile(t, root, "CLAUDE.local.md"), "LOCAL_BODY")
	assert.FileExists(t, filepath.Join(root, "AGENTS.md"))
}
