package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/userscope"
)

const userPresetConfigFmt = `version = "5.0"
name = "me"
presets = [%s]

[[hooks]]
event = "Stop"
[[hooks.hooks]]
command = "echo done"

[permissions]
allow = ["Bash(git status)"]
`

func userPresetFixture() map[string]string {
	files := userFixture()
	files["commands/ship.md"] = "---\ndescription: Ship a release\n---\nShip it.\n"
	files["rules/scoped.md"] = "---\npaths:\n  - \"**/*.go\"\n---\n# Scoped\nOnly for Go.\n"
	return files
}

func quotedPresets(names ...string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = fmt.Sprintf("%q", name)
	}
	return strings.Join(quoted, ", ")
}

// underAnyRoot reports whether path lies below one of the roots.
func underAnyRoot(path string, roots ...string) bool {
	for _, root := range roots {
		if isUnderBaseDir(root, path) {
			return true
		}
	}
	return false
}

func noEnv(string) string { return "" }

// TestUser_EveryPresetMapsOrIsReportedUnsupported renders each built-in preset on
// its own. A preset either writes every output it maps to an absolute path below
// the home directory, with nothing of unknown provenance dropped, or is
// reported unsupported with a reason.
func TestUser_EveryPresetMapsOrIsReportedUnsupported(t *testing.T) {
	names := config.IndividualPresetNames()
	require.NotEmpty(t, names)

	supported := 0
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			warnings := quietWarnings(t)
			home, gen := newUserHome(t, fmt.Sprintf(userPresetConfigFmt, quotedPresets(name)), userPresetFixture())
			gen.SetUserEnv(noEnv)
			layout, resolveErr := userscope.Resolve(name, home, noEnv)

			if resolveErr != nil {
				require.True(t, userscope.IsUnsupported(resolveErr), "%v", resolveErr)
				var unsupported *userscope.UnsupportedError
				require.ErrorAs(t, resolveErr, &unsupported)
				assert.NotEmpty(t, unsupported.Reason)

				plan, err := gen.GenerateUser("")
				require.NoError(t, err)
				assert.Empty(t, plan.Writes)
				assert.Empty(t, plan.Merges)
				assert.Contains(t, strings.Join(*warnings, "\n"), "preset "+name+" has no documented user-level location")
				assert.Contains(t, strings.Join(*warnings, "\n"), unsupported.Reason)
				return
			}
			supported++

			plan, err := gen.GenerateUser("")
			require.NoError(t, err)
			written := append(append([]string{}, plan.Writes...), plan.Merges...)
			require.NotEmpty(t, written, "a supported preset writes something")
			for _, path := range written {
				assert.True(t, filepath.IsAbs(path), "%s is absolute", path)
				assert.True(t, isUnderBaseDir(home, path), "%s is below the home directory", path)
				assert.FileExists(t, path)
			}
			// Everything the preset rendered but did not write is of a kind the layout
			// knows and the tool has no user-level folder for.
			for _, entry := range plan.Unmapped {
				preset, rel, _ := strings.Cut(entry, ": ")
				assert.Equal(t, name, preset)
				row, ok := layout.Classify(rel)
				assert.True(t, ok, "%s: no layout row covers this output, so its provenance is unknown", entry)
				assert.Empty(t, row.To, "%s is dropped although the layout maps it", entry)
			}
		})
	}
	assert.Greater(t, supported, 40, "most presets document a user scope")
}

// TestUser_HomeEnvOverrides relocates every tool home that has a variable and
// checks the files follow it, and that clean takes them out of it again.
func TestUser_HomeEnvOverrides(t *testing.T) {
	var relocatable []string
	for _, name := range config.IndividualPresetNames() {
		layout, err := userscope.Resolve(name, "/home/probe", func(string) string { return "/tools/probe" })
		if err == nil && layout.RelocatedHome != "" {
			relocatable = append(relocatable, name)
		}
	}
	require.Contains(t, relocatable, "codex")
	require.Contains(t, relocatable, "hermes")

	for _, name := range relocatable {
		t.Run(name, func(t *testing.T) {
			quietWarnings(t)
			home, gen := newUserHome(t, fmt.Sprintf(userPresetConfigFmt, quotedPresets(name)), userPresetFixture())
			tools := t.TempDir()
			getenv := func(string) string { return tools }
			gen.SetUserEnv(getenv)
			before := filesUnder(t, home)

			plan, err := gen.GenerateUser("")
			require.NoError(t, err)
			relocated := 0
			for _, path := range append(append([]string{}, plan.Writes...), plan.Merges...) {
				assert.True(t, underAnyRoot(path, home, tools), "%s is below the home directory or the relocated home", path)
				if isUnderBaseDir(tools, path) {
					relocated++
				}
			}
			assert.Positive(t, relocated, "the home variable moves the tool's files")

			// A fresh run with the same variable finds the files it wrote, then clean removes them.
			again := loadUserGenerator(t, home)
			again.SetUserEnv(getenv)
			plan, err = again.PlanUser("")
			require.NoError(t, err)
			assert.Empty(t, plan.Skips, "files written by the first run are recognised as its own")
			assert.Empty(t, plan.Stale)

			clean := loadUserGenerator(t, home)
			clean.SetUserEnv(getenv)
			_, err = clean.Clean("", CleanOptions{KeepGitignore: true})
			require.NoError(t, err)
			assert.Equal(t, before, filesUnder(t, home))
			assert.Empty(t, filesUnder(t, tools))
		})
	}
}

func TestUser_RelativeHomeOverrideIsIgnored(t *testing.T) {
	quietWarnings(t)
	home, gen := newUserHome(t, fmt.Sprintf(userPresetConfigFmt, quotedPresets("codex")), userPresetFixture())
	gen.SetUserEnv(func(string) string { return "relative/codex" })

	plan, err := gen.GenerateUser("")
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(home, ".codex", "AGENTS.md"))
	for _, path := range plan.Writes {
		assert.True(t, isUnderBaseDir(home, path), path)
	}
	assert.NoDirExists(t, "relative")
}

func TestUser_RelativeHomeIsRefused(t *testing.T) {
	_, gen := newUserHome(t, fmt.Sprintf(userPresetConfigFmt, quotedPresets("codex")), userPresetFixture())
	gen.config.BaseDir = "relative/home"
	_, err := gen.PlanUser("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute")
}

// snapshotHome maps every regular file below root to its content, and lists the
// directories.
func snapshotHome(t *testing.T, root string) (files map[string]string, dirs []string) {
	t.Helper()
	files = map[string]string{}
	for _, rel := range filesUnder(t, root) {
		files[rel] = readFileString(t, filepath.Join(root, filepath.FromSlash(rel)))
	}
	require.NoError(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() && path != root {
			rel, relErr := filepath.Rel(root, path)
			require.NoError(t, relErr)
			dirs = append(dirs, filepath.ToSlash(rel))
		}
		return nil
	}))
	sort.Strings(dirs)
	return files, dirs
}

// TestUser_GenerateThenCleanRestoresHome runs every supported preset together
// against a home that already holds hand-authored files, then cleans. The home
// must hold exactly the files it started with, and every directory the run
// created must be empty or gone.
func TestUser_GenerateThenCleanRestoresHome(t *testing.T) {
	quietWarnings(t)
	layouts, _, err := userscope.All("/home/probe", noEnv)
	require.NoError(t, err)
	names := userscope.Supported(layouts)
	require.Greater(t, len(names), 40)

	home, gen := newUserHome(t, fmt.Sprintf(userPresetConfigFmt, quotedPresets(names...)), userPresetFixture())
	gen.SetUserEnv(noEnv)
	writeTree(t, home, map[string]string{
		".claude/CLAUDE.md":                  "my own notes\n",
		".claude/settings.json":              "{\n  \"model\": \"opus\"\n}\n",
		".claude/skills/mine/SKILL.md":       "---\nname: mine\n---\nMine.\n",
		".codex/hooks.json":                  "{\n  \"hooks\": {\n    \"SessionStart\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"mine\"}]}]\n  }\n}\n",
		".gemini/settings.json":              "{\n  \"theme\": \"dark\"\n}\n",
		".config/opencode/agents/own.md":     "my agent\n",
		".cursor/hooks.json":                 "{\n  \"version\": 1\n}\n",
		".agents/skills/shared-mine/NOTE.md": "keep\n",
	})
	beforeFiles, beforeDirs := snapshotHome(t, home)

	plan, err := gen.GenerateUser("")
	require.NoError(t, err)
	require.NotEmpty(t, plan.Writes)
	require.NotEmpty(t, plan.Merges)
	afterFiles, _ := snapshotHome(t, home)
	assert.Greater(t, len(afterFiles), len(beforeFiles)+20)
	assert.Equal(t, "my own notes\n", afterFiles[".claude/CLAUDE.md"], "a hand-authored file is never replaced")
	assert.Contains(t, afterFiles[".claude/settings.json"], `"model": "opus"`)

	// A second run is a no-op.
	second, err := loadUserGenerator(t, home).GenerateUser("")
	require.NoError(t, err)
	assert.Empty(t, second.Stale)
	secondFiles, _ := snapshotHome(t, home)
	assert.Equal(t, afterFiles, secondFiles)

	_, err = loadUserGenerator(t, home).Clean("", CleanOptions{KeepGitignore: true})
	require.NoError(t, err)

	cleanFiles, cleanDirs := snapshotHome(t, home)
	assert.Equal(t, beforeFiles, cleanFiles, "clean restores every file exactly")
	known := map[string]bool{}
	for _, dir := range beforeDirs {
		known[dir] = true
	}
	for _, dir := range cleanDirs {
		if known[dir] {
			continue
		}
		entries, readErr := os.ReadDir(filepath.Join(home, filepath.FromSlash(dir)))
		require.NoError(t, readErr)
		assert.Empty(t, entries, "%s was created by generate and holds something after clean", dir)
	}
}
