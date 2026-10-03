package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const agentsMDScopesMCPGoldenFile = "testdata/agents_md_off_scopes_mcp_golden.json"

// TestAgentsMD_FlagOffOutputUnchangedWithScopesAndMCP extends the flag-off golden
// to a project with [[scopes]] and an MCP server, the paths the shared outputs
// touch in scope runs and the settings documents. The golden was recorded from
// the release before the flag; refresh with UPDATE_GOLDEN=1 only for an intended
// rendering change.
func TestAgentsMD_FlagOffOutputUnchangedWithScopesAndMCP(t *testing.T) {
	all := config.IndividualPresetNames()
	scopeBlock := "\n[profiles]\napi = [\"api\"]\n\n[[scopes]]\npath = \"packages/api\"\nprofile = \"api\"\npresets = [" +
		quotedList(all) + "]\n"
	cases := []struct{ name, flag string }{
		{name: "flag absent", flag: ""},
		{name: "flag false", flag: "agents_md = false\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeAgentsMDProject(t, root, tc.flag+agentsMDConfig(all, "", agentsMDMCPServer+scopeBlock))
			writeAgentsMDFile(t, root, ".ai-rulez/rules/go-style.md", agentsMDGoStyleRule)
			writeAgentsMDFile(t, root, ".ai-rulez/domains/api/rules/api-style.md", "# Api Style\n\nAPI_STYLE_BODY\n")
			runAgentsMDGenerate(t, root)
			got := agentsMDSnapshot(t, root)

			if os.Getenv("UPDATE_GOLDEN") != "" && tc.flag == "" {
				data, err := json.MarshalIndent(got, "", "  ")
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(agentsMDScopesMCPGoldenFile, append(data, '\n'), 0o644))
			}
			raw, err := os.ReadFile(agentsMDScopesMCPGoldenFile)
			require.NoError(t, err)
			var want map[string]string
			require.NoError(t, json.Unmarshal(raw, &want))
			assert.Equal(t, want, got)
		})
	}
}

func quotedList(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = `"` + item + `"`
	}
	return strings.Join(quoted, ", ")
}

func TestAgentsMD_TargetedAtOtherOwnerReachesSharedAgentsMD(t *testing.T) {
	// xum is not configured, but it is a default owner of AGENTS.md, so an item
	// targeted at it stays in the file the codex preset renders.
	root := t.TempDir()
	writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig([]string{"codex"}, "", ""))
	writeAgentsMDFile(t, root, ".ai-rulez/rules/targeted.md", "---\ntargets: [\"xum\"]\n---\nTARGETED_BODY\n")
	runAgentsMDGenerate(t, root)

	assert.Contains(t, readAgentsMDFile(t, root, "AGENTS.md"), "TARGETED_BODY")
}

func TestAgentsMD_HeaderHashModesApplyToSharedOutputs(t *testing.T) {
	cases := []struct {
		name       string
		header     string
		wantSource bool
		wantHashes bool
	}{
		{"full", "", true, true},
		{"content", "\n[header]\nhashes = \"content\"\n", false, true},
		{"none", "\n[header]\nhashes = \"none\"\n", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig([]string{"codex", "claude"}, "", tc.header))
			runAgentsMDGenerate(t, root)

			for _, rel := range []string{"AGENTS.md", ".agents/skills/alpha/SKILL.md"} {
				content := readAgentsMDFile(t, root, rel)
				assert.Equal(t, tc.wantHashes, strings.Contains(content, "Content-Hash: blake3:"), rel)
				assert.Equal(t, tc.wantSource, strings.Contains(content, "Source-Hash: blake3:"), rel)
			}
		})
	}
}

func TestAgentsMD_ToggleKeepsHandWrittenSkills(t *testing.T) {
	root := t.TempDir()
	handWritten := []string{".codex/skills/mine/SKILL.md", ".agents/skills/mine/SKILL.md"}
	presets := []string{"codex"}

	writeAgentsMDProject(t, root, agentsMDConfig(presets, "", ""))
	runAgentsMDGenerate(t, root)
	for _, rel := range handWritten {
		writeAgentsMDFile(t, root, rel, "---\nname: mine\ndescription: mine\n---\nMINE\n")
	}

	for _, flag := range []string{"agents_md = true\n", "", "agents_md = true\n", ""} {
		writeAgentsMDProject(t, root, flag+agentsMDConfig(presets, "", ""))
		runAgentsMDGenerate(t, root)
		for _, rel := range handWritten {
			assert.Equal(t, "---\nname: mine\ndescription: mine\n---\nMINE\n", readAgentsMDFile(t, root, rel),
				"%q with flag %q", rel, flag)
		}
	}
}

func TestAgentsMD_LocalOverrideFileUnaffected(t *testing.T) {
	root := t.TempDir()
	writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig([]string{"codex"}, "", ""))
	writeAgentsMDFile(t, root, ".ai-rulez/local/rules/mine.md", "# Mine\n\nLOCAL_BODY\n")
	runAgentsMDGenerate(t, root)

	assert.Contains(t, readAgentsMDFile(t, root, "AGENTS.override.md"), "LOCAL_BODY")
	assert.NotContains(t, readAgentsMDFile(t, root, "AGENTS.md"), "LOCAL_BODY")
}

// TestAgentsMD_SharedOutputsWinOverOtherPresets covers presets that write the
// shared paths without relying on them: their copy must not overwrite the shared
// file whatever the preset names sort like.
func TestAgentsMD_SharedOutputsWinOverOtherPresets(t *testing.T) {
	cases := []struct {
		name    string
		presets []string
	}{
		{"codex and gemini", []string{"codex", "gemini"}},
		{"codex and cursor", []string{"codex", "cursor"}},
		{"codex, cursor, gemini and antigravity", []string{"codex", "cursor", "gemini", "antigravity"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig(tc.presets, "", ""))
			runAgentsMDGenerate(t, root)

			skill := readAgentsMDFile(t, root, ".agents/skills/alpha/SKILL.md")
			assert.Contains(t, skill, "name: alpha")
			agents := readAgentsMDFile(t, root, "AGENTS.md")
			_, agentsHash := extractStoredHashes(filepath.Join(root, "AGENTS.md"))
			_, skillHash := extractStoredHashes(filepath.Join(root, ".agents", "skills", "alpha", "SKILL.md"))
			assert.NotEmpty(t, agentsHash)
			assert.Equal(t, agentsHash, skillHash, "both come from the shared render")
			assert.Contains(t, agents, "ALWAYS_BODY")
		})
	}
}

func TestDropShadowedByShared(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "proj")
	file := func(rel string) config.OutputFile { return config.OutputFile{Path: filepath.Join(base, rel)} }
	build := func() map[string][]config.OutputFile {
		return map[string][]config.OutputFile{
			"zzz-custom": {file("AGENTS.md"), file("other.md")},
			"cursor":     {file(".agents/skills/alpha/SKILL.md"), file(".cursor/rules/a.mdc")},
		}
	}
	paths := func(outputs []config.OutputFile) []string {
		var out []string
		for _, o := range outputs {
			rel, _ := filepath.Rel(base, o.Path)
			out = append(out, filepath.ToSlash(rel))
		}
		return out
	}

	cases := []struct {
		name             string
		agentsMD, skills bool
		wantCustom       []string
		wantCursor       []string
	}{
		{"both", true, true, []string{"other.md"}, []string{".cursor/rules/a.mdc"}},
		{"agents only", true, false, []string{"other.md"}, []string{".agents/skills/alpha/SKILL.md", ".cursor/rules/a.mdc"}},
		{"skills only", false, true, []string{"AGENTS.md", "other.md"}, []string{".cursor/rules/a.mdc"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			all := build()
			dropShadowedByShared(all, base, tc.agentsMD, tc.skills, nil)
			assert.Equal(t, tc.wantCustom, paths(all["zzz-custom"]))
			assert.Equal(t, tc.wantCursor, paths(all["cursor"]))
		})
	}
}

func TestDropOwnSharedOutputs_ComparesCleanedPaths(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "proj")
	codex, ok := config.SharedOutputConsumerFor("codex")
	require.True(t, ok)

	untidy := func(rel string) config.OutputFile {
		return config.OutputFile{Path: base + string(filepath.Separator) + "." + string(filepath.Separator) + filepath.FromSlash(rel)}
	}
	cases := []struct {
		name     string
		consumer config.SharedOutputConsumer
		input    []config.OutputFile
		want     int
	}{
		{"codex AGENTS.md and own skills", codex,
			[]config.OutputFile{untidy("AGENTS.md"), untidy(".codex/skills/a/SKILL.md"), untidy(".agents/skills/a/SKILL.md"), untidy("keep.md")}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kept := dropOwnSharedOutputs(tc.input, base, "codex", tc.consumer, nil)
			assert.Len(t, kept, tc.want)
		})
	}
}
