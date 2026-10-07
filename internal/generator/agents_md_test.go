package generator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const agentsMDGoldenFile = "testdata/agents_md_off_golden.json"

var agentsMDFixture = map[string]string{
	"rules/always.md":              "# Always\n\nALWAYS_BODY\n",
	"context/overview.md":          "# Overview\n\nOVERVIEW_BODY\n",
	"skills/alpha/SKILL.md":        "---\ndescription: Alpha skill\n---\nALPHA_BODY\n",
	"skills/alpha/references/r.md": "REFERENCE_BODY\n",
	"skills/beta/SKILL.md":         "---\ndescription: Beta skill\n---\nBETA_BODY\n",
	"agents/helper.md":             "---\ndescription: Helper agent\n---\nHELPER_BODY\n",
}

func agentsMDConfig(presets []string, flag string, extra string) string {
	quoted := make([]string, len(presets))
	for i, p := range presets {
		quoted[i] = strconv.Quote(p)
	}
	return "version = \"4.0\"\nname = \"shared\"\ngitignore = false\npresets = [" + strings.Join(quoted, ", ") + "]\n" +
		flag + extra
}

func writeAgentsMDProject(t *testing.T, root string, cfgTOML string) {
	t.Helper()
	files := map[string]string{"config.toml": cfgTOML}
	for k, v := range agentsMDFixture {
		files[k] = v
	}
	for rel, content := range files {
		path := filepath.Join(root, ".ai-rulez", filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
}

func runAgentsMDGenerate(t *testing.T, root string, opts ...config.LoadOption) {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root, opts...)
	require.NoError(t, err)
	require.NoError(t, NewGenerator(cfg).Generate(""))
}

// agentsMDSnapshot maps each generated file below root (the .ai-rulez source tree
// excluded) to its sha256.
func agentsMDSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".ai-rulez" {
				return filepath.SkipDir
			}
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		sum := sha256.Sum256(data)
		tree[rel] = hex.EncodeToString(sum[:])
		return nil
	}))
	return tree
}

func sharedManifestFiles(t *testing.T, root string) []string {
	t.Helper()
	return readManifestFile(nil, filepath.Join(root, ".ai-rulez", generatedManifestName)).Files
}

// TestAgentsMD_FlagOffOutputUnchanged pins the default: with agents_md absent or
// false, all 14 presets render the bytes the release before the flag rendered.
// The golden hashes were recorded from that release; refresh them with
// UPDATE_GOLDEN=1 only for an intended rendering change.
func TestAgentsMD_FlagOffOutputUnchanged(t *testing.T) {
	cases := []struct{ name, flag string }{
		{name: "flag absent", flag: ""},
		{name: "flag false", flag: "agents_md = false\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			// Top-level keys must precede tables, so the flag goes before presets
			// by living in the preamble of the config.
			writeAgentsMDProject(t, root, tc.flag+agentsMDConfig(config.IndividualPresetNames(), "", ""))
			runAgentsMDGenerate(t, root)
			got := agentsMDSnapshot(t, root)

			if os.Getenv("UPDATE_GOLDEN") != "" && tc.flag == "" {
				data, err := json.MarshalIndent(got, "", "  ")
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(agentsMDGoldenFile, append(data, '\n'), 0o644))
			}
			raw, err := os.ReadFile(agentsMDGoldenFile)
			require.NoError(t, err)
			var want map[string]string
			require.NoError(t, json.Unmarshal(raw, &want))
			assert.Equal(t, want, got)
		})
	}
}

func TestAgentsMD_SharedOutputsWrittenOnce(t *testing.T) {
	perToolSkills := []string{".codex/skills", ".opencode/skills", ".xum/skills"}
	cases := []struct {
		name    string
		presets []string
	}{
		{"codex only", []string{"codex"}},
		{"codex and opencode", []string{"codex", "opencode"}},
		{"all four", []string{"codex", "opencode", "amp", "xum"}},
		{"amp only", []string{"amp"}},
	}

	var agentsHashes, agentsBodies []string
	for _, tc := range cases {
		root := t.TempDir()
		writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig(tc.presets, "", ""))
		runAgentsMDGenerate(t, root)

		for _, skill := range []string{"alpha", "beta"} {
			path := filepath.Join(root, ".agents", "skills", skill, "SKILL.md")
			assert.FileExists(t, path, tc.name)
		}
		assert.FileExists(t, filepath.Join(root, ".agents", "skills", "alpha", "references", "r.md"), tc.name)
		for _, dir := range perToolSkills {
			assert.NoDirExists(t, filepath.Join(root, filepath.FromSlash(dir)), "%s: %s", tc.name, dir)
		}

		tree := agentsMDSnapshot(t, root)
		var agentsFiles []string
		for rel := range tree {
			if filepath.Base(rel) == "AGENTS.md" {
				agentsFiles = append(agentsFiles, rel)
			}
		}
		assert.Equal(t, []string{"AGENTS.md"}, agentsFiles, tc.name)

		_, sourceHash := extractStoredHashes(filepath.Join(root, "AGENTS.md"))
		require.NotEmpty(t, sourceHash, tc.name)
		agentsHashes = append(agentsHashes, sourceHash)
		body, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
		require.NoError(t, err)
		agentsBodies = append(agentsBodies, string(body))
		assert.Contains(t, string(body), "ALWAYS_BODY", tc.name)
		assert.Contains(t, string(body), "OVERVIEW_BODY", tc.name)

		skillBody, err := os.ReadFile(filepath.Join(root, ".agents", "skills", "alpha", "SKILL.md"))
		require.NoError(t, err)
		assert.Contains(t, string(skillBody), "name: alpha", tc.name)
		_, skillHash := extractStoredHashes(filepath.Join(root, ".agents", "skills", "alpha", "SKILL.md"))
		assert.Equal(t, agentsHashes[0], skillHash, "shared outputs share one source hash: %s", tc.name)
	}

	for i := range agentsHashes {
		assert.Equal(t, agentsHashes[0], agentsHashes[i], "AGENTS.md Source-Hash must not depend on the preset list: %s", cases[i].name)
		assert.Equal(t, agentsBodies[0], agentsBodies[i], "AGENTS.md must be byte-identical across preset lists: %s", cases[i].name)
	}
}

func TestAgentsMD_SecondRunIsIdempotent(t *testing.T) {
	root := t.TempDir()
	writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig([]string{"codex", "opencode", "amp", "xum"}, "", ""))

	runAgentsMDGenerate(t, root)
	first := agentsMDSnapshot(t, root)
	stamp, err := os.Stat(filepath.Join(root, "AGENTS.md"))
	require.NoError(t, err)

	runAgentsMDGenerate(t, root)
	assert.Equal(t, first, agentsMDSnapshot(t, root))
	again, err := os.Stat(filepath.Join(root, "AGENTS.md"))
	require.NoError(t, err)
	assert.Equal(t, stamp.ModTime(), again.ModTime(), "an unchanged shared file must not be rewritten")
}

func TestAgentsMD_ToggleCleansAndRestores(t *testing.T) {
	cases := []struct {
		name         string
		presets      []string
		wantSharedOn bool // .agents/skills survives the flag turning off (codex and amp write it themselves)
	}{
		{"without amp", []string{"codex", "opencode", "xum"}, true},
		{"with amp", []string{"codex", "opencode", "xum", "amp"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			skill := func(dir string) string { return filepath.Join(root, filepath.FromSlash(dir), "alpha", "SKILL.md") }
			perTool := []string{".opencode/skills", ".xum/skills"}

			writeAgentsMDProject(t, root, agentsMDConfig(tc.presets, "", ""))
			runAgentsMDGenerate(t, root)
			for _, dir := range perTool {
				require.FileExists(t, skill(dir))
			}
			offTree := agentsMDSnapshot(t, root)

			writeAgentsMDProject(t, root, "agents_md = true\n"+agentsMDConfig(tc.presets, "", ""))
			runAgentsMDGenerate(t, root)
			for _, dir := range perTool {
				assert.NoDirExists(t, filepath.Join(root, filepath.FromSlash(dir)), "flag on removes %s", dir)
			}
			assert.FileExists(t, skill(".agents/skills"))
			assert.Contains(t, sharedManifestFiles(t, root), ".agents/skills/alpha/SKILL.md")

			writeAgentsMDProject(t, root, agentsMDConfig(tc.presets, "", ""))
			runAgentsMDGenerate(t, root)
			for _, dir := range perTool {
				assert.FileExists(t, skill(dir), "flag off restores %s", dir)
			}
			if tc.wantSharedOn {
				assert.FileExists(t, skill(".agents/skills"))
			} else {
				assert.NoDirExists(t, filepath.Join(root, ".agents", "skills"))
			}
			assert.Equal(t, offTree, agentsMDSnapshot(t, root), "off, on, off ends where off began")
		})
	}
}

func TestAgentsMD_ScopeGetsNestedAgentsMD(t *testing.T) {
	root := t.TempDir()
	cfgTOML := "agents_md = true\n" + agentsMDConfig([]string{"codex", "opencode"}, "", `
[profiles]
api = ["api"]

[[scopes]]
path = "packages/api"
profile = "api"
presets = ["codex", "opencode"]
`)
	writeAgentsMDProject(t, root, cfgTOML)
	apiRule := filepath.Join(root, ".ai-rulez", "domains", "api", "rules", "api-style.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(apiRule), 0o755))
	require.NoError(t, os.WriteFile(apiRule, []byte("# Api Style\n\nAPI_STYLE_BODY\n"), 0o644))

	runAgentsMDGenerate(t, root)

	var agentsFiles []string
	for rel := range agentsMDSnapshot(t, root) {
		if filepath.Base(rel) == "AGENTS.md" {
			agentsFiles = append(agentsFiles, rel)
		}
	}
	sort.Strings(agentsFiles)
	assert.Equal(t, []string{"AGENTS.md", "packages/api/AGENTS.md"}, agentsFiles)
	nested, err := os.ReadFile(filepath.Join(root, "packages", "api", "AGENTS.md"))
	require.NoError(t, err)
	assert.Contains(t, string(nested), "API_STYLE_BODY")
	assert.NoDirExists(t, filepath.Join(root, "packages", "api", ".codex", "skills"))
}
