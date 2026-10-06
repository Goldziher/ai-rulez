package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const claudeOnlyConfig = "version = \"4.0\"\nname = \"me\"\npresets = [\"claude\"]\n"

func writeUserManifest(t *testing.T, home string, files ...string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"version": "1", "files": files})
	require.NoError(t, err)
	writeTree(t, filepath.Join(home, ".config", "ai-rulez"), map[string]string{generatedManifestName: string(data)})
}

func TestUser_TamperedManifestEntriesAreNeverDeleted(t *testing.T) {
	quietWarnings(t)
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim.md")
	tests := []struct {
		name  string
		entry func(home string) string
		file  func(home string) string
	}{
		{"parent traversal out of the home directory", func(home string) string {
			rel, err := filepath.Rel(home, victim)
			require.NoError(t, err)
			return filepath.ToSlash(rel)
		}, func(string) string { return victim }},
		{"dot-dot inside an allowed root", func(string) string { return ".claude/skills/../../keep/me.md" },
			func(home string) string { return filepath.Join(home, "keep", "me.md") }},
		{"inside the home directory but no layout row", func(string) string { return "documents/notes.md" },
			func(home string) string { return filepath.Join(home, "documents", "notes.md") }},
		{"tool folder file no row covers", func(string) string { return ".claude/notes.md" },
			func(home string) string { return filepath.Join(home, ".claude", "notes.md") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			home, gen := newUserHome(t, claudeOnlyConfig, userFixture())
			file := tt.file(home)
			require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
			require.NoError(t, os.WriteFile(file, []byte("# hand written\n"), 0o644))
			writeUserManifest(t, home, tt.entry(home))

			// Act
			plan, err := gen.GenerateUser("")

			// Assert
			require.NoError(t, err)
			assert.FileExists(t, file)
			assert.NotContains(t, plan.Stale, file)
		})
	}
}

func TestUser_StaleHandWrittenFileWithoutBannerSurvivesWhateverTheHeaderMode(t *testing.T) {
	quietWarnings(t)
	home, gen := newUserHome(t, claudeOnlyConfig+"\n[header]\nhashes = \"none\"\n", userFixture())
	hand := filepath.Join(home, ".claude", "agents", "old.md")
	writeTree(t, home, map[string]string{".claude/agents/old.md": "---\nname: old\n---\nmine\n"})
	writeUserManifest(t, home, ".claude/agents/old.md")

	_, err := gen.GenerateUser("")

	require.NoError(t, err)
	assert.FileExists(t, hand)
}

func TestUser_SymlinkEscapeIsNeverFollowedByDeletion(t *testing.T) {
	quietWarnings(t)
	home, gen := newUserHome(t, claudeOnlyConfig, userFixture())
	outside := t.TempDir()
	victim := filepath.Join(outside, "res.json")
	require.NoError(t, os.WriteFile(victim, []byte("{}"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(home, ".claude", "skills", "old"))
	writeUserManifest(t, home, ".claude/skills/old/res.json")

	_, err := gen.GenerateUser("")

	require.NoError(t, err)
	assert.FileExists(t, victim, "a manifest entry behind a symlink out of the home directory is not removed")
}

func TestUser_EmptyDirPruneDoesNotFollowSymlinksOut(t *testing.T) {
	quietWarnings(t)
	home, gen := newUserHome(t, claudeOnlyConfig, userFixture())
	outside := t.TempDir()
	emptied := filepath.Join(outside, "empty")
	require.NoError(t, os.MkdirAll(emptied, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755))
	testutil.SymlinkOrSkip(t, emptied, filepath.Join(home, ".claude", "skills", "link"))

	gen.removeEmptyDir(filepath.Join(home, ".claude", "skills", "link"))

	assert.DirExists(t, emptied)
}

func TestConvertToRelativePath_NeverFallsBackToTheBaseName(t *testing.T) {
	// Arrange: a base the path cannot be made relative to (as across Windows volumes).
	g := &Generator{config: &config.Config{BaseDir: "relative/base"}}
	abs := filepath.Join(string(filepath.Separator), "elsewhere", ".claude", "rules", "x.md")

	// Act
	rel, err := g.relativeToBase(abs)
	got := g.convertToRelativePath(abs)

	// Assert
	require.Error(t, err)
	assert.Empty(t, rel)
	assert.Equal(t, abs, got, "the unexpressible path stays whole instead of collapsing to x.md")
}

func TestUser_OnlyConfiguredPresetsRelocateScope(t *testing.T) {
	quietWarnings(t)
	home, gen := newUserHome(t, claudeOnlyConfig, userFixture())
	tools := t.TempDir()
	gen.SetUserEnv(func(string) string { return tools })

	_, _, err := gen.resolveUserLayouts()

	require.NoError(t, err)
	assert.Empty(t, gen.userHomes, "codex, hermes and the rest are not configured, so their homes are not writable scope")
	assert.False(t, gen.withinScope(filepath.Join(tools, "AGENTS.md")))
	assert.True(t, gen.withinScope(filepath.Join(home, ".claude", "x")))
}

func TestUser_EachPresetRendersIntoItsOwnStageDirectory(t *testing.T) {
	quietWarnings(t)
	_, gen := newUserHome(t, userConfigTOML, userFixture())
	stage := t.TempDir()
	cfg := *gen.config
	cfg.Content = &config.ContentTree{}
	cfg.UserScope = true
	cfg.Run = config.NewRunState()
	layouts, _, err := gen.resolveUserLayouts()
	require.NoError(t, err)

	rendered, err := gen.renderUserPresets(&cfg, stage, userPresets(&cfg, layouts))

	require.NoError(t, err)
	require.NotEmpty(t, rendered)
	for preset, outputs := range rendered {
		for _, output := range outputs {
			assert.True(t, isUnderBaseDir(userStageDir(stage, preset), output.Path),
				"%s renders %s outside its own stage directory", preset, output.Path)
		}
	}
}
