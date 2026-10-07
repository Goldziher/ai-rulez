package generator

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const localContentConfig = `version = "5.0"
name = "t"
presets = ["%s"]
gitignore = true
agents_md = false

[header]
hashes = "full"

[profiles]
me = ["mine"]
`

// localContentProject writes a project with one shared rule and a full machine
// local tree: root rule, context, skill, agent, command, and a "mine" domain with
// a rule, a skill and a context file.
func localContentProject(t *testing.T, preset string) string {
	t.Helper()
	dir := t.TempDir()
	c := filepath.Join(dir, ".ai-rulez")
	seedLocalFile(t, filepath.Join(c, "config.toml"), strings.Replace(localContentConfig, "%s", preset, 1))
	seedLocalFile(t, filepath.Join(c, "rules", "shared.md"), "---\npriority: high\n---\n\nShared body.\n")
	seedLocalFile(t, filepath.Join(c, "local", "rules", "mine-rule.md"), "---\npriority: low\n---\n\nLOCAL_RULE.\n")
	seedLocalFile(t, filepath.Join(c, "local", "context", "mine-ctx.md"), "LOCAL_CONTEXT.\n")
	seedLocalFile(t, filepath.Join(c, "local", "skills", "mine-skill", "SKILL.md"), "---\ndescription: d\n---\n\nLOCAL_SKILL.\n")
	seedLocalFile(t, filepath.Join(c, "local", "agents", "mine-agent.md"), "---\nname: mine-agent\ndescription: d\n---\n\nLOCAL_AGENT.\n")
	seedLocalFile(t, filepath.Join(c, "local", "commands", "mine-cmd.md"), "---\ndescription: d\n---\n\nLOCAL_COMMAND.\n")
	seedLocalFile(t, filepath.Join(c, "local", "domains", "mine", "rules", "dom-rule.md"), "---\npriority: low\n---\n\nLOCAL_DOMAIN_RULE.\n")
	seedLocalFile(t, filepath.Join(c, "local", "domains", "mine", "skills", "dom-skill", "SKILL.md"), "---\ndescription: d\n---\n\nLOCAL_DOMAIN_SKILL.\n")
	return dir
}

func generateLocalIn(t *testing.T, dir, profile string, opts ...config.LoadOption) {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), dir, opts...)
	require.NoError(t, err)
	_, err = NewGenerator(cfg).GenerateFiles(profile)
	require.NoError(t, err)
}

// treeSnapshot maps every file outside .git and the local source tree to its content.
func treeSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, ".git/") || strings.HasPrefix(rel, ".ai-rulez/local/") {
			return nil
		}
		data, err := os.ReadFile(p)
		require.NoError(t, err)
		out[rel] = string(data)
		return nil
	}))
	return out
}

func manifestFiles(t *testing.T, dir, name string) []string {
	t.Helper()
	var m struct {
		Files []string `json:"files"`
	}
	require.NoError(t, jsonUnmarshal(readRel(t, dir, ".ai-rulez/"+name), &m))
	return m.Files
}

func TestLocalContent_PerPreset(t *testing.T) {
	tests := []struct {
		name   string
		preset string
		// files maps each local-only output to the body text it must carry.
		files map[string]string
	}{
		{"claude", "claude", map[string]string{
			".claude/rules/mine-rule.local.md":   "LOCAL_RULE.",
			".claude/rules/dom-rule.local.md":    "LOCAL_DOMAIN_RULE.",
			"CLAUDE.local.md":                    "LOCAL_CONTEXT.",
			".claude/skills/mine-skill/SKILL.md": "LOCAL_SKILL.",
			".claude/skills/dom-skill/SKILL.md":  "LOCAL_DOMAIN_SKILL.",
			".claude/agents/mine-agent.md":       "LOCAL_AGENT.",
			".claude/skills/mine-cmd/SKILL.md":   "LOCAL_COMMAND.",
		}},
		{"cursor", "cursor", map[string]string{
			".cursor/rules/mine-rule.local.mdc":  "LOCAL_RULE.",
			".cursor/rules/dom-rule.local.mdc":   "LOCAL_DOMAIN_RULE.",
			".agents/skills/mine-skill/SKILL.md": "LOCAL_SKILL.",
			".agents/skills/dom-skill/SKILL.md":  "LOCAL_DOMAIN_SKILL.",
			".cursor/agents/mine-agent.md":       "LOCAL_AGENT.",
			".cursor/commands/mine-cmd.md":       "LOCAL_COMMAND.",
		}},
		{"copilot", "copilot", map[string]string{
			".github/instructions/mine-rule.local.instructions.md": "LOCAL_RULE.",
			".github/instructions/dom-rule.local.instructions.md":  "LOCAL_DOMAIN_RULE.",
			".github/instructions/ai-rulez.local.instructions.md":  "LOCAL_CONTEXT.",
			".github/skills/mine-skill/SKILL.md":                   "LOCAL_SKILL.",
			".github/skills/dom-skill/SKILL.md":                    "LOCAL_DOMAIN_SKILL.",
			".github/agents/mine-agent.agent.md":                   "LOCAL_AGENT.",
			".github/prompts/mine-cmd.prompt.md":                   "LOCAL_COMMAND.",
		}},
		{"antigravity", "antigravity", map[string]string{
			".agents/rules/mine-rule.local.md":   "LOCAL_RULE.",
			".agents/rules/dom-rule.local.md":    "LOCAL_DOMAIN_RULE.",
			".agents/rules/ai-rulez.local.md":    "LOCAL_CONTEXT.",
			".agents/skills/mine-skill/SKILL.md": "LOCAL_SKILL.",
			".agents/skills/dom-skill/SKILL.md":  "LOCAL_DOMAIN_SKILL.",
			".agents/agents/mine-agent.md":       "LOCAL_AGENT.",
		}},
		{"codex", "codex", map[string]string{
			"AGENTS.override.md":                 "LOCAL_CONTEXT.",
			".agents/skills/mine-skill/SKILL.md": "LOCAL_SKILL.",
			".agents/skills/dom-skill/SKILL.md":  "LOCAL_DOMAIN_SKILL.",
			".codex/agents/mine-agent.toml":      "LOCAL_AGENT.",
			".agents/skills/mine-cmd/SKILL.md":   "LOCAL_COMMAND.",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := localContentProject(t, tt.preset)

			// Act: the "me" profile selects the local "mine" domain.
			generateLocalIn(t, dir, "me")

			// Assert: every local output exists with its content, is recorded in the
			// local manifest only, and is not in the shared one.
			local := manifestFiles(t, dir, ".generated-manifest.local.json")
			shared := manifestFiles(t, dir, ".generated-manifest.json")
			for rel, body := range tt.files {
				if strings.Contains(rel, "dom-") && tt.preset == "codex" && strings.HasSuffix(rel, ".local.md") {
					continue
				}
				assert.Contains(t, readRel(t, dir, rel), body, rel)
				assert.Contains(t, local, rel, "local manifest lists %s", rel)
				assert.NotContains(t, shared, rel, "shared manifest must not list %s", rel)
			}
			for _, f := range shared {
				assert.NotContains(t, readRel(t, dir, f), "LOCAL_", "shared output %s carries local content", f)
			}

			// The drift guard classifies all of them as local-only and reports no drift.
			cfg, err := config.LoadConfig(context.Background(), dir)
			require.NoError(t, err)
			plan, err := NewGenerator(cfg).DryRun("me")
			require.NoError(t, err)
			for rel := range tt.files {
				assert.Contains(t, plan, "local-only: "+rel)
			}
			assert.False(t, containsPrefix(plan, "drift:"), "plan: %v", plan)
		})
	}
}

func TestLocalContent_DomainFollowsProfile(t *testing.T) {
	// Arrange
	dir := localContentProject(t, "claude")

	// Act: the default profile does not name the "mine" domain.
	generateLocalIn(t, dir, "")

	// Assert
	assert.FileExists(t, filepath.Join(dir, ".claude", "skills", "mine-skill", "SKILL.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "skills", "dom-skill", "SKILL.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "rules", "dom-rule.local.md"))
}

func TestLocalContent_TargetsAreHonoured(t *testing.T) {
	// Arrange: a local skill that targets claude only, rendered for cursor.
	dir := localContentProject(t, "cursor")
	seedLocalFile(t, filepath.Join(dir, ".ai-rulez", "local", "skills", "claude-only", "SKILL.md"),
		"---\ndescription: d\ntargets:\n  - claude\n---\n\nCLAUDE_ONLY.\n")

	// Act
	generateLocalIn(t, dir, "")

	// Assert
	assert.NoFileExists(t, filepath.Join(dir, ".agents", "skills", "claude-only", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "mine-skill", "SKILL.md"))
}

func TestLocalContent_IDCollisionWithSharedItemIsAnError(t *testing.T) {
	tests := []struct {
		name          string
		shared, local string
	}{
		{"skill vs skill", "skills/dup/SKILL.md", "local/skills/dup/SKILL.md"},
		{"agent vs agent", "agents/dup.md", "local/agents/dup.md"},
		{"command vs shared skill", "skills/dup/SKILL.md", "local/commands/dup.md"},
		{"skill vs shared command", "commands/dup.md", "local/skills/dup/SKILL.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := localContentProject(t, "claude")
			body := "---\ndescription: d\nname: dup\n---\n\nbody\n"
			seedLocalFile(t, filepath.Join(dir, ".ai-rulez", filepath.FromSlash(tt.shared)), body)
			seedLocalFile(t, filepath.Join(dir, ".ai-rulez", filepath.FromSlash(tt.local)), body)
			cfg, err := config.LoadConfig(context.Background(), dir)
			require.NoError(t, err)

			// Act
			_, err = NewGenerator(cfg).GenerateFiles("")

			// Assert: the error names both sources, and nothing local was written.
			require.Error(t, err)
			assert.Contains(t, err.Error(), "collides")
			assert.Contains(t, err.Error(), filepath.ToSlash(tt.shared)[strings.LastIndex(tt.shared, "/")+1:])
			assert.Contains(t, err.Error(), "local")
			assert.NoFileExists(t, filepath.Join(dir, ".claude", "skills", "mine-skill", "SKILL.md"))
		})
	}
}

func TestLocalContent_TeammateParity(t *testing.T) {
	// Arrange: the same project with and without a local tree.
	withLocal := localContentProject(t, "claude")
	without := t.TempDir()
	seedLocalFile(t, filepath.Join(without, ".ai-rulez", "config.toml"), strings.Replace(localContentConfig, "%s", "claude", 1))
	seedLocalFile(t, filepath.Join(without, ".ai-rulez", "rules", "shared.md"), "---\npriority: high\n---\n\nShared body.\n")

	// Act: this machine generates with its local tree; a teammate has none.
	generateLocalIn(t, withLocal, "")
	generateLocalIn(t, without, "")

	// Assert: every output the teammate has is byte-identical here, and the only
	// extras are local-only files.
	mine, theirs := treeSnapshot(t, withLocal), treeSnapshot(t, without)
	localSet := map[string]bool{}
	for _, f := range manifestFiles(t, withLocal, ".generated-manifest.local.json") {
		localSet[f] = true
	}
	for rel, content := range theirs {
		if rel == ".gitignore" || rel == ".ai-rulez/.generated-manifest.json" {
			continue
		}
		assert.Equal(t, content, mine[rel], rel)
	}
	var extra []string
	for rel := range mine {
		if _, shared := theirs[rel]; !shared {
			extra = append(extra, rel)
		}
	}
	sort.Strings(extra)
	for _, rel := range extra {
		assert.True(t, localSet[rel] || rel == ".ai-rulez/.generated-manifest.local.json", "unexpected extra output %s", rel)
	}
	assert.Equal(t, readRel(t, without, ".ai-rulez/.generated-manifest.json"), readRel(t, withLocal, ".ai-rulez/.generated-manifest.json"))

	// And --no-local reproduces the teammate view without touching local files.
	generateLocalIn(t, withLocal, "", config.WithoutLocal())
	assert.FileExists(t, filepath.Join(withLocal, ".claude", "skills", "mine-skill", "SKILL.md"))
}

func TestLocalContent_IgnoredPerClone(t *testing.T) {
	// Arrange
	dir := localContentProject(t, "claude")
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	require.NoError(t, exec.Command("git", "-C", dir, "init", "-q").Run()) //nolint:gosec // test

	// Act
	generateLocalIn(t, dir, "")

	// Assert: the machine-specific skill is excluded in this clone only; the stable
	// ".local." names are covered by the shared pattern.
	exclude := readRel(t, dir, ".git/info/exclude")
	assert.Contains(t, exclude, "/.claude/skills/mine-skill/SKILL.md")
	assert.Contains(t, exclude, "/.claude/agents/mine-agent.md")
	ignore := readRel(t, dir, ".gitignore")
	assert.NotContains(t, ignore, "mine-skill")
	assert.Contains(t, ignore, ".claude/rules/*.local.*")
}

func TestLocalContent_CleanRemovesLocalOutputs(t *testing.T) {
	// Arrange
	dir := localContentProject(t, "claude")
	generateLocalIn(t, dir, "me")
	outputs := manifestFiles(t, dir, ".generated-manifest.local.json")
	require.NotEmpty(t, outputs)

	// Act
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	_, err = NewGenerator(cfg).Clean("me", CleanOptions{})
	require.NoError(t, err)

	// Assert: local outputs and their manifest are gone; the sources stay.
	for _, rel := range outputs {
		assert.NoFileExists(t, filepath.Join(dir, filepath.FromSlash(rel)))
	}
	assert.NoFileExists(t, filepath.Join(dir, ".ai-rulez", ".generated-manifest.local.json"))
	assert.FileExists(t, filepath.Join(dir, ".ai-rulez", "local", "skills", "mine-skill", "SKILL.md"))
}

func TestLocalContent_RemovedSourceDeletesOutput(t *testing.T) {
	// Arrange
	dir := localContentProject(t, "claude")
	generateLocalIn(t, dir, "")
	skill := filepath.Join(dir, ".claude", "skills", "mine-skill", "SKILL.md")
	require.FileExists(t, skill)

	// Act
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez", "local", "skills")))
	generateLocalIn(t, dir, "")

	// Assert
	assert.NoFileExists(t, skill)
}

func TestLocalContent_NeverEmittedInScopeRuns(t *testing.T) {
	// Arrange: a monorepo with a local skill and a local rule in a scope's domain.
	root := writeScopedRulesProject(t, "split", nil)
	cfgDir := filepath.Join(root, ".ai-rulez")
	seedLocalFile(t, filepath.Join(cfgDir, "local", "skills", "mine-skill", "SKILL.md"), "---\ndescription: d\n---\n\nLOCAL_SKILL.\n")
	seedLocalFile(t, filepath.Join(cfgDir, "local", "domains", "api", "rules", "mine.md"), "LOCAL_DOMAIN_RULE.\n")

	// Act
	generateLocalIn(t, root, "")

	// Assert: the root run carries the local skill, no scope directory carries any.
	assert.FileExists(t, filepath.Join(root, ".claude", "skills", "mine-skill", "SKILL.md"))
	require.NoError(t, filepath.Walk(filepath.Join(root, "packages"), func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(p)
		require.NoError(t, readErr)
		assert.NotContains(t, string(data), "LOCAL_", p)
		return nil
	}))
}

func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }

func TestLocalContent_ReportsItemsAPresetCannotPlace(t *testing.T) {
	tests := []struct {
		name    string
		preset  string
		dropped []string // labels expected for the preset; nil means everything is placed
	}{
		{"claude writes a file per item", "claude", nil},
		{"gemini has no commands folder", "gemini", []string{"command mine-cmd"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := localContentProject(t, tt.preset)
			cfg, err := config.LoadConfig(context.Background(), dir)
			require.NoError(t, err)
			g := NewGenerator(cfg)
			all, err := config.GeneratePresets(cfg)
			require.NoError(t, err)
			known := map[string]bool{}
			for _, outs := range all {
				for _, o := range outs {
					known[o.Path] = true
				}
			}

			// Act
			dropped := g.droppedItems(cfg, perItemContent(cfg.LocalContent), known, map[string]bool{tt.preset: true})

			// Assert
			assert.Equal(t, tt.dropped, dropped[tt.preset])
		})
	}
}
