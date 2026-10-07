package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const handWritten = "# My notes\n\nDo not lose me.\n"

func newProjectGenerator(t *testing.T, dir string) *Generator {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())
	return NewGenerator(cfg)
}

func TestGenerate_RefusesAnExistingFileItCannotProveItWrote(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := hashesProject(t, "")
	claude := filepath.Join(dir, "CLAUDE.md")
	require.NoError(t, os.WriteFile(claude, []byte(handWritten), 0o644))

	// Act
	err := newProjectGenerator(t, dir).Generate("default")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CLAUDE.md")
	assert.Contains(t, err.Error(), "ai-rulez convert")
	assert.Contains(t, err.Error(), "--force")
	data, rerr := os.ReadFile(claude)
	require.NoError(t, rerr)
	assert.Equal(t, handWritten, string(data), "the hand-written file is untouched")
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "skills", "alpha", "SKILL.md"), "a refused run writes nothing")
}

func TestGenerate_ForceOverwritesAnUnprovenFile(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := hashesProject(t, "")
	claude := filepath.Join(dir, "CLAUDE.md")
	require.NoError(t, os.WriteFile(claude, []byte(handWritten), 0o644))
	gen := newProjectGenerator(t, dir)
	gen.SetOverwriteUnowned(true)

	// Act
	err := gen.Generate("default")

	// Assert
	require.NoError(t, err)
	data, rerr := os.ReadFile(claude)
	require.NoError(t, rerr)
	assert.Contains(t, string(data), "GENERATED FILE")
}

func TestGenerate_RewritesFilesItProvablyWrote(t *testing.T) {
	quietWarnings(t)
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
	}{
		{"a second run over its own output", func(t *testing.T, dir string) {}},
		{"a file whose header was stripped but the manifest lists it", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("stripped\n"), 0o644))
		}},
		{"an empty file", func(t *testing.T, dir string) {
			require.NoError(t, os.Remove(filepath.Join(dir, ".ai-rulez", generatedManifestName)))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), nil, 0o644))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := hashesProject(t, "")
			generateHashesProject(t, dir)
			tt.setup(t, dir)

			// Act
			err := newProjectGenerator(t, dir).Generate("default")

			// Assert
			require.NoError(t, err)
			data, rerr := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
			require.NoError(t, rerr)
			assert.Contains(t, string(data), "GENERATED FILE")
		})
	}
}

func TestDryRunAndCheck_ReportTheRefusal(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := hashesProject(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(handWritten), 0o644))
	gen := newProjectGenerator(t, dir)

	// Act
	lines, err := gen.DryRun("default")
	blocked := gen.DryRunBlocked()
	drift, checkErr := newProjectGenerator(t, dir).CheckDrift("default")

	// Assert
	require.NoError(t, err)
	assert.Contains(t, lines, "blocked: CLAUDE.md (an existing file ai-rulez did not write)")
	assert.NotContains(t, lines, "write-file: CLAUDE.md")
	require.Error(t, blocked)
	assert.Contains(t, blocked.Error(), "CLAUDE.md")
	require.NoError(t, checkErr)
	assert.Contains(t, drift, Drift{Path: "CLAUDE.md", Kind: DriftBlocked})
}

func TestGenerate_AdoptsAFileConvertImported(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := hashesProject(t, "")
	claude := filepath.Join(dir, "CLAUDE.md")
	require.NoError(t, os.WriteFile(claude, []byte(handWritten), 0o644))
	require.NoError(t, WriteConvertRecord(filepath.Join(dir, ".ai-rulez"), map[string][]byte{"CLAUDE.md": []byte(handWritten)}))

	// Act
	err := newProjectGenerator(t, dir).Generate("default")

	// Assert
	require.NoError(t, err)
	data, rerr := os.ReadFile(claude)
	require.NoError(t, rerr)
	assert.Contains(t, string(data), "GENERATED FILE")
}

func TestGenerate_DoesNotAdoptAConvertedFileEditedSince(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := hashesProject(t, "")
	claude := filepath.Join(dir, "CLAUDE.md")
	require.NoError(t, WriteConvertRecord(filepath.Join(dir, ".ai-rulez"), map[string][]byte{"CLAUDE.md": []byte(handWritten)}))
	require.NoError(t, os.WriteFile(claude, []byte(handWritten+"\nadded after convert\n"), 0o644))

	// Act
	err := newProjectGenerator(t, dir).Generate("default")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CLAUDE.md")
}

func linkedProject(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	writeAgentsMDProject(t, dir, agentsMDConfig([]string{"claude", "codex"}, "", ""))
	return dir
}

func TestGenerate_ASymlinkOntoAGeneratedPathIsLeftAlone(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := linkedProject(t)
	testutil.SymlinkOrSkip(t, "AGENTS.md", filepath.Join(dir, "CLAUDE.md"))

	// Act
	err := newProjectGenerator(t, dir).Generate("default")

	// Assert
	require.NoError(t, err)
	info, lerr := os.Lstat(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, lerr)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the user's link survives")
	data, rerr := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, rerr)
	assert.Contains(t, string(data), "GENERATED FILE", "the target holds its own generated content")
	manifest, merr := os.ReadFile(filepath.Join(dir, ".ai-rulez", generatedManifestName))
	require.NoError(t, merr)
	assert.NotContains(t, string(manifest), `"CLAUDE.md"`, "a link the user made is not recorded as generated")
}

func TestClean_NeverRemovesAUsersSymlink(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := linkedProject(t)
	testutil.SymlinkOrSkip(t, "AGENTS.md", filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, newProjectGenerator(t, dir).Generate("default"))

	// Act
	_, err := newProjectGenerator(t, dir).Clean("default", CleanOptions{RemoveEdited: true})

	// Assert
	require.NoError(t, err)
	info, lerr := os.Lstat(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, lerr, "the link is the user's")
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
	assert.NoFileExists(t, filepath.Join(dir, "AGENTS.md"), "the generated target is removed with the rest")
}

func TestKeepReason_NamesTheRealReason(t *testing.T) {
	quietWarnings(t)
	tests := []struct {
		name         string
		dropManifest bool
		want         string
	}{
		{"listed in the manifest", false, "the generated manifest lists it"},
		{"not listed in the manifest", true, "is not in the generated manifest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := hashesProject(t, "")
			generateHashesProject(t, dir)
			claude := filepath.Join(dir, "CLAUDE.md")
			require.NoError(t, os.WriteFile(claude, []byte("stripped of its header\n"), 0o644))
			if tt.dropManifest {
				require.NoError(t, os.Remove(filepath.Join(dir, ".ai-rulez", generatedManifestName)))
			}

			// Act
			reason := newProjectGenerator(t, dir).keepReason(claude)

			// Assert
			assert.Contains(t, reason, tt.want)
		})
	}
}

func TestClean_KeepsTheGeneratedFileThatReplacedAConvertedOriginal(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir := hashesProject(t, "")
	claude := filepath.Join(dir, "CLAUDE.md")
	require.NoError(t, os.WriteFile(claude, []byte(handWritten), 0o644))
	require.NoError(t, WriteConvertRecord(filepath.Join(dir, ".ai-rulez"), map[string][]byte{"CLAUDE.md": []byte(handWritten)}))
	require.NoError(t, newProjectGenerator(t, dir).Generate("default"))

	// Act
	plan, err := newProjectGenerator(t, dir).Clean("default", CleanOptions{RemoveEdited: true})

	// Assert
	require.NoError(t, err)
	assert.FileExists(t, claude, "the user must not end up with neither the original nor the generated file")
	for _, f := range plan.Files {
		assert.NotEqual(t, claude, f)
	}
	assert.NoFileExists(t, filepath.Join(dir, ".claude", "skills", "alpha", "SKILL.md"), "other outputs are still cleaned")
}
