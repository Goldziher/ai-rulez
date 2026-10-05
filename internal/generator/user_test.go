package generator

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
)

const userConfigTOML = `version = "4.0"
name = "me"
presets = ["claude", "codex", "gemini", "opencode", "cursor", "copilot", "pi", "baz"]

[profiles]
work = ["work"]

[[hooks]]
event = "Stop"
[[hooks.hooks]]
command = "echo done"

[permissions]
allow = ["Bash(git status)"]
`

const userSkill = "---\nname: my-skill\ndescription: Does a personal thing. Use when I ask for my thing.\n---\nSkill body.\n"

func userFixture() map[string]string {
	return map[string]string{
		"skills/my-skill/SKILL.md":                userSkill,
		"rules/personal.md":                       "# Personal\nAlways be brief.\n",
		"agents/helper.md":                        "---\nname: helper\ndescription: helps\n---\nHelp.\n",
		"domains/work/skills/work-skill/SKILL.md": "---\nname: work-skill\ndescription: Work only. Use when doing work tasks.\n---\nWork.\n",
		"domains/work/rules/work.md":              "# Work\nUse the work tracker.\n",
	}
}

// newUserHome writes a user config below <home>/.config/ai-rulez and returns the
// home directory and a Generator in user scope.
func newUserHome(t *testing.T, configTOML string, files map[string]string) (string, *Generator) {
	t.Helper()
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "ai-rulez")
	writeTree(t, configDir, map[string]string{"config.toml": configTOML})
	writeTree(t, configDir, files)
	return home, loadUserGenerator(t, home)
}

func loadUserGenerator(t *testing.T, home string) *Generator {
	t.Helper()
	cfg, err := config.LoadConfigFromFile(context.Background(), filepath.Join(home, ".config", "ai-rulez"), config.WithoutLocal())
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())
	cfg.BaseDir = home
	gen := NewGenerator(cfg)
	gen.SetUserScope()
	return gen
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
}

// filesUnder lists the regular files below root as slash-separated relative paths.
func filesUnder(t *testing.T, root string, skip ...string) []string {
	t.Helper()
	var out []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		rel, relErr := filepath.Rel(root, path)
		require.NoError(t, relErr)
		rel = filepath.ToSlash(rel)
		for _, prefix := range skip {
			if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if !d.IsDir() {
			out = append(out, rel)
		}
		return nil
	}))
	sort.Strings(out)
	return out
}

func quietWarnings(t *testing.T) *[]string {
	t.Helper()
	var warnings []string
	restore := rulefiles.SetWarnSink(func(msg string, _ ...any) { warnings = append(warnings, msg) })
	t.Cleanup(restore)
	return &warnings
}

var expectedUserFiles = []string{
	".agents/skills/my-skill/SKILL.md",
	".claude/CLAUDE.md",
	".claude/agents/helper.md",
	".claude/rules/personal.md",
	".claude/settings.json",
	".claude/skills/my-skill/SKILL.md",
	".codex/AGENTS.md",
	".codex/agents/helper.toml",
	".codex/hooks.json",
	".config/opencode/AGENTS.md",
	".config/opencode/agents/helper.md",
	".config/opencode/opencode.json",
	".config/opencode/plugins/ai-rulez-hooks.js",
	".config/opencode/skills/my-skill/SKILL.md",
	".copilot/agents/helper.agent.md",
	".copilot/copilot-instructions.md",
	".copilot/hooks/ai-rulez.json",
	".copilot/instructions/personal.instructions.md",
	".copilot/skills/my-skill/SKILL.md",
	".cursor/agents/helper.md",
	".cursor/hooks.json",
	".gemini/GEMINI.md",
	".gemini/agents/helper.md",
	".gemini/settings.json",
	".pi/agent/AGENTS.md",
	".pi/agent/agents/helper.md",
	".pi/agent/extensions/ai-rulez-hooks.ts",
	".pi/agent/skills/my-skill/SKILL.md",
}

func TestUser_WritesOnlyDocumentedLocations(t *testing.T) {
	warnings := quietWarnings(t)
	home, gen := newUserHome(t, userConfigTOML, userFixture())

	plan, err := gen.GenerateUser("")
	require.NoError(t, err)

	assert.Equal(t, expectedUserFiles, filesUnder(t, home, ".config/ai-rulez/agents", ".config/ai-rulez/domains",
		".config/ai-rulez/rules", ".config/ai-rulez/skills", ".config/ai-rulez/config.toml",
		".config/ai-rulez/.generated-manifest.json", ".config/ai-rulez/.generated-manifest.local.json"))
	assert.NotEmpty(t, plan.Writes)
	assert.FileExists(t, filepath.Join(home, ".config", "ai-rulez", generatedManifestName))
	assert.NoFileExists(t, filepath.Join(home, "CLAUDE.md"), "project-shaped files never land in the home root")
	assert.NoFileExists(t, filepath.Join(home, ".gitignore"))
	assert.Contains(t, strings.Join(*warnings, "\n"), "preset baz has no documented user-level location")

	// Work-domain content is outside the default profile.
	assert.NoFileExists(t, filepath.Join(home, ".claude", "skills", "work-skill", "SKILL.md"))

	settings := readFileString(t, filepath.Join(home, ".claude", "settings.json"))
	assert.Contains(t, settings, "Bash(git status)")
	assert.Contains(t, settings, "echo done")
	assert.NotContains(t, settings, "mcpServers", "project-only keys are not written at user level")
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func TestUser_ProfilePerRole(t *testing.T) {
	quietWarnings(t)
	home, gen := newUserHome(t, userConfigTOML, userFixture())

	_, err := gen.GenerateUser("work")
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(home, ".claude", "skills", "work-skill", "SKILL.md"))
	assert.Contains(t, readFileString(t, filepath.Join(home, ".claude", "rules", "work.md")), "work tracker")

	// Switching the machine back to the default role removes the work content again.
	_, err = loadUserGenerator(t, home).GenerateUser("")
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(home, ".claude", "skills", "work-skill", "SKILL.md"))
	assert.NoDirExists(t, filepath.Join(home, ".claude", "skills", "work-skill"))
}

func TestUser_PlanWritesNothing(t *testing.T) {
	quietWarnings(t)
	home, gen := newUserHome(t, userConfigTOML, userFixture())
	before := filesUnder(t, home)

	plan, err := gen.PlanUser("")
	require.NoError(t, err)

	assert.Equal(t, before, filesUnder(t, home))
	var listed []string
	for _, p := range append(append([]string{}, plan.Writes...), plan.Merges...) {
		rel, relErr := filepath.Rel(home, p)
		require.NoError(t, relErr)
		listed = append(listed, filepath.ToSlash(rel))
	}
	sort.Strings(listed)
	assert.Equal(t, expectedUserFiles, listed, "the plan names every target path")
}

func TestUser_NeverClobbersHandAuthoredFiles(t *testing.T) {
	quietWarnings(t)
	home, gen := newUserHome(t, userConfigTOML, userFixture())
	handClaude := "my own notes\n"
	handSkill := "---\nname: my-skill\ndescription: mine\n---\nHand written.\n"
	handSettings := "{\n  \"model\": \"opus\",\n  \"permissions\": {\"allow\": [\"Bash(make)\"]}\n}\n"
	writeTree(t, home, map[string]string{
		".claude/CLAUDE.md":                handClaude,
		".claude/skills/my-skill/SKILL.md": handSkill,
		".claude/settings.json":            handSettings,
		".codex/AGENTS.md":                 "codex notes\n",
	})

	plan, err := gen.GenerateUser("")
	require.NoError(t, err)

	assert.Equal(t, handClaude, readFileString(t, filepath.Join(home, ".claude", "CLAUDE.md")))
	assert.Equal(t, handSkill, readFileString(t, filepath.Join(home, ".claude", "skills", "my-skill", "SKILL.md")))
	assert.Equal(t, "codex notes\n", readFileString(t, filepath.Join(home, ".codex", "AGENTS.md")))
	var skipped []string
	for _, skip := range plan.Skips {
		skipped = append(skipped, filepath.Base(skip.Path))
	}
	assert.ElementsMatch(t, []string{"CLAUDE.md", "SKILL.md", "AGENTS.md"}, skipped)

	settings := readFileString(t, filepath.Join(home, ".claude", "settings.json"))
	for _, want := range []string{`"model": "opus"`, "Bash(make)", "Bash(git status)", "echo done"} {
		assert.Contains(t, settings, want)
	}
	manifest := readFileString(t, filepath.Join(home, ".config", "ai-rulez", generatedManifestName))
	assert.NotContains(t, manifest, ".claude/CLAUDE.md")
	assert.NotContains(t, manifest, ".claude/skills/my-skill/SKILL.md", "a skipped file is not recorded, so clean cannot remove it")
	assert.NotContains(t, manifest, ".codex/AGENTS.md")

	// clean removes what ai-rulez wrote and leaves everything else.
	_, err = gen.Clean("", CleanOptions{KeepGitignore: true})
	require.NoError(t, err)
	assert.Equal(t, handClaude, readFileString(t, filepath.Join(home, ".claude", "CLAUDE.md")))
	assert.Equal(t, handSkill, readFileString(t, filepath.Join(home, ".claude", "skills", "my-skill", "SKILL.md")))
	assert.Equal(t, "codex notes\n", readFileString(t, filepath.Join(home, ".codex", "AGENTS.md")))
	cleaned := readFileString(t, filepath.Join(home, ".claude", "settings.json"))
	assert.Contains(t, cleaned, `"model": "opus"`)
	assert.Contains(t, cleaned, "Bash(make)")
	assert.NotContains(t, cleaned, "Bash(git status)")
	assert.NotContains(t, cleaned, "echo done")
}

func TestUser_ManifestCleanRemovesExactlyTheWrittenFiles(t *testing.T) {
	quietWarnings(t)
	home, gen := newUserHome(t, userConfigTOML, userFixture())
	writeTree(t, home, map[string]string{".claude/notes.md": "keep me\n", ".claude/skills/other/SKILL.md": "---\nname: other\n---\nmine\n"})

	_, err := gen.GenerateUser("")
	require.NoError(t, err)
	first := map[string]string{}
	for _, rel := range expectedUserFiles {
		first[rel] = readFileString(t, filepath.Join(home, filepath.FromSlash(rel)))
	}

	// A second run changes nothing.
	_, err = loadUserGenerator(t, home).GenerateUser("")
	require.NoError(t, err)
	for rel, content := range first {
		assert.Equal(t, content, readFileString(t, filepath.Join(home, filepath.FromSlash(rel))), rel)
	}

	plan, err := loadUserGenerator(t, home).Clean("", CleanOptions{KeepGitignore: true})
	require.NoError(t, err)
	assert.NotEmpty(t, plan.Files)

	assert.Equal(t, []string{
		".claude/notes.md",
		".claude/skills/other/SKILL.md",
	}, filesUnder(t, home, ".config/ai-rulez"), "only files ai-rulez did not write remain")
	assert.NoFileExists(t, filepath.Join(home, ".config", "ai-rulez", generatedManifestName))
	assert.DirExists(t, filepath.Join(home, ".claude"), "directories ai-rulez does not own are never removed")
	assert.DirExists(t, filepath.Join(home, ".config", "ai-rulez"), "the user config is never touched")
	assert.FileExists(t, filepath.Join(home, ".config", "ai-rulez", "config.toml"))
}

func TestUser_StaleFilesAreRemovedAndEditedOnesKept(t *testing.T) {
	quietWarnings(t)
	home, gen := newUserHome(t, userConfigTOML, userFixture())
	_, err := gen.GenerateUser("")
	require.NoError(t, err)

	// The skill leaves the config; the generated agent is replaced by a hand-written file.
	require.NoError(t, os.RemoveAll(filepath.Join(home, ".config", "ai-rulez", "skills")))
	require.NoError(t, os.RemoveAll(filepath.Join(home, ".config", "ai-rulez", "agents")))
	handAgent := "---\nname: helper\n---\nmy own agent\n"
	require.NoError(t, os.WriteFile(filepath.Join(home, ".claude", "agents", "helper.md"), []byte(handAgent), 0o644))

	_, err = loadUserGenerator(t, home).GenerateUser("")
	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(home, ".claude", "skills", "my-skill"))
	assert.NoFileExists(t, filepath.Join(home, ".agents", "skills", "my-skill", "SKILL.md"))
	assert.Equal(t, handAgent, readFileString(t, filepath.Join(home, ".claude", "agents", "helper.md")),
		"a generated file the user has since replaced is not removed as stale")
	assert.NoFileExists(t, filepath.Join(home, ".gemini", "agents", "helper.md"), "an unedited stale agent is removed")
}

func TestUser_SymlinkHandling(t *testing.T) {
	quietWarnings(t)

	t.Run("a directory symlinked inside the home directory is followed", func(t *testing.T) {
		home, gen := newUserHome(t, "version = \"4.0\"\nname = \"me\"\npresets = [\"claude\"]\n", userFixture())
		dotfiles := filepath.Join(home, "dotfiles", "claude")
		require.NoError(t, os.MkdirAll(dotfiles, 0o755))
		require.NoError(t, os.Symlink(dotfiles, filepath.Join(home, ".claude")))

		_, err := gen.GenerateUser("")
		require.NoError(t, err)
		assert.FileExists(t, filepath.Join(dotfiles, "skills", "my-skill", "SKILL.md"))
	})

	t.Run("a symlink out of the home directory is refused", func(t *testing.T) {
		home, gen := newUserHome(t, "version = \"4.0\"\nname = \"me\"\npresets = [\"claude\"]\n", userFixture())
		outside := t.TempDir()
		require.NoError(t, os.Symlink(outside, filepath.Join(home, ".claude")))

		_, err := gen.GenerateUser("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "outside the home directory")
		assert.Empty(t, filesUnder(t, outside))
	})

	t.Run("a symlinked file is left alone", func(t *testing.T) {
		home, gen := newUserHome(t, "version = \"4.0\"\nname = \"me\"\npresets = [\"claude\"]\n", userFixture())
		target := filepath.Join(home, "elsewhere.md")
		require.NoError(t, os.WriteFile(target, []byte("shared\n"), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude", "rules"), 0o755))
		require.NoError(t, os.Symlink(target, filepath.Join(home, ".claude", "rules", "personal.md")))

		plan, err := gen.GenerateUser("")
		require.NoError(t, err)
		assert.Equal(t, "shared\n", readFileString(t, target))
		require.Len(t, plan.Skips, 1)
		assert.Equal(t, "is a symlink", plan.Skips[0].Reason)
	})
}

func TestUser_WarnsAboutSkillsLoadedTwice(t *testing.T) {
	quietWarnings(t)
	home, gen := newUserHome(t, userConfigTOML, userFixture())
	project := t.TempDir()
	writeTree(t, project, map[string]string{".claude/skills/my-skill/SKILL.md": "---\nname: my-skill\n---\nproject\n"})
	gen.SetProjectDir(project)

	plan, err := gen.PlanUser("")
	require.NoError(t, err)
	joined := strings.Join(plan.Warnings, "\n")
	assert.Contains(t, joined, "opencode reads ~/.config/opencode/skills, ~/.claude/skills, ~/.agents/skills")
	assert.Contains(t, joined, "cursor reads ~/.agents/skills, ~/.claude/skills")
	assert.Contains(t, joined, `skill "my-skill" exists in the project (.claude/skills/my-skill) and at user level (~/.claude/skills/my-skill): Claude Code runs the user-level skill`)

	// The home directory is not a project.
	gen.SetProjectDir(home)
	plan, err = gen.PlanUser("")
	require.NoError(t, err)
	assert.NotContains(t, strings.Join(plan.Warnings, "\n"), "exists in the project")
}
