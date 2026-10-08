package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initFromProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"CLAUDE.md":                    "# Project\n\nUse Go.\n",
		".cursor/rules/ts.mdc":         "---\nglobs: '**/*.ts'\n---\nStrict mode.\n",
		".claude/skills/lint/SKILL.md": "---\nname: lint\ndescription: Lint\n---\nRun lint.\n",
	}
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	return dir
}

func setInitFlags(t *testing.T, from string) {
	t.Helper()
	fromFlag, autoYes, initForce, initConfigDirArg = from, true, true, ""
	t.Cleanup(func() { fromFlag, autoYes, initForce, initConfigDirArg = "", false, false, "" })
}

func TestInitFrom_RunsConvert(t *testing.T) {
	tests := []struct {
		name      string
		from      string
		wantFiles []string
		wantNot   []string
	}{
		{
			name:      "auto reads every detected source",
			from:      "auto",
			wantFiles: []string{"context/claude.md", "rules/ts.md", "skills/lint/SKILL.md", "config.toml"},
		},
		{
			name:      "project paths select what is read",
			from:      ".cursor,CLAUDE.md",
			wantFiles: []string{"context/claude.md", "rules/ts.md", "config.toml"},
			wantNot:   []string{"skills/lint/SKILL.md"},
		},
		{
			name:      "an importer name works too",
			from:      "native",
			wantFiles: []string{"context/claude.md", "rules/ts.md", "skills/lint/SKILL.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := initFromProject(t)
			chdir(t, dir)
			setInitFlags(t, tt.from)

			// Act
			runInit(InitCmd, nil)

			// Assert
			for _, f := range tt.wantFiles {
				assert.FileExists(t, filepath.Join(dir, ".ai-rulez", filepath.FromSlash(f)))
			}
			for _, f := range tt.wantNot {
				assert.NoFileExists(t, filepath.Join(dir, ".ai-rulez", filepath.FromSlash(f)))
			}
		})
	}
}

func TestInitFrom_ReplacesAnExistingDirectoryOnlyWhenTheImportWorks(t *testing.T) {
	// Arrange
	dir := initFromProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez", "rules"), 0o755))
	stale := filepath.Join(dir, ".ai-rulez", "rules", "stale.md")
	require.NoError(t, os.WriteFile(stale, []byte("stale\n"), 0o644))
	chdir(t, dir)
	setInitFlags(t, "auto")

	// Act
	runInit(InitCmd, nil)

	// Assert
	assert.NoFileExists(t, stale, "the old directory is replaced by the import")
	assert.FileExists(t, filepath.Join(dir, ".ai-rulez", "context", "claude.md"))
}

func TestPreviewInitImport(t *testing.T) {
	t.Run("nothing to import fails before anything is removed", func(t *testing.T) {
		dir := t.TempDir()
		chdir(t, dir)
		setInitFlags(t, "auto")

		err := previewInitImport(context.Background(), dir)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "no importable files")
	})
	t.Run("a blocked scan fails and writes nothing", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("Token: ghp_abcdefghijklmnopqrstuvwxyz0123456789\n"), 0o644))
		chdir(t, dir)
		setInitFlags(t, "auto")

		err := previewInitImport(context.Background(), dir)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "security scan")
		assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez"))
	})
	t.Run("a good import writes nothing either", func(t *testing.T) {
		dir := initFromProject(t)
		chdir(t, dir)
		setInitFlags(t, "auto")

		require.NoError(t, previewInitImport(context.Background(), dir))

		assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez"))
	})
}
