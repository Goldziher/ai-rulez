package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func llmsProject(t *testing.T, llmsBlock string) string {
	t.Helper()
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(
		"version = \"5.0\"\nname = \"llms\"\npresets = [\"claude\", \"llms-txt\"]\ngitignore = false\n\n"+llmsBlock+
			"\n[[roles]]\nname = \"eng\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "rules", "style.md"), []byte("# Style\n\nUse tabs.\n"), 0o644))
	return dir
}

func TestLLMsTxt_FullFileDroppedByConfigIsRemovedWithoutWarning(t *testing.T) {
	quietWarnings(t)
	dir := llmsProject(t, "[llms_txt]\nfull = true\n")
	require.NoError(t, newProjectGenerator(t, dir).Generate("default"))
	require.FileExists(t, filepath.Join(dir, "llms-full.txt"))

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte(
		"version = \"5.0\"\nname = \"llms\"\npresets = [\"claude\", \"llms-txt\"]\ngitignore = false\n\n[llms_txt]\nfull = false\n"), 0o644))
	g := newProjectGenerator(t, dir)
	matcher := g.outputMatcher(nil)

	assert.True(t, matcher.matches("llms-full.txt"))
	require.NoError(t, g.Generate("default"))
	assert.NoFileExists(t, filepath.Join(dir, "llms-full.txt"))
}

func TestLLMsTxt_MatcherHonoursConfiguredDir(t *testing.T) {
	dir := llmsProject(t, "[llms_txt]\ndir = \"docs/sub\"\nfull = true\n")
	m := newProjectGenerator(t, dir).outputMatcher(nil)
	assert.True(t, m.matches("docs/sub/llms-full.txt"))
	assert.True(t, m.matches("docs/sub/llms.txt"))
}

func TestLLMsTxt_RoleRunKeepsCommittedFiles(t *testing.T) {
	quietWarnings(t)
	dir := llmsProject(t, "[llms_txt]\nfull = true\n")
	require.NoError(t, newProjectGenerator(t, dir).Generate("default"))

	g := newProjectGenerator(t, dir)
	require.NoError(t, g.SetRole("eng"))
	require.NoError(t, g.Generate("default"))

	assert.FileExists(t, filepath.Join(dir, "llms.txt"))
	assert.FileExists(t, filepath.Join(dir, "llms-full.txt"))
}

func TestClean_IncludeEditedRemovesAnEditedBannerlessPresetOutput(t *testing.T) {
	quietWarnings(t)
	dir := llmsProject(t, "[llms_txt]\nfull = true\n")
	require.NoError(t, newProjectGenerator(t, dir).Generate("default"))
	f, err := os.OpenFile(filepath.Join(dir, "llms.txt"), os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("edited\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	_, err = newProjectGenerator(t, dir).Clean("default", CleanOptions{RemoveEdited: true})

	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(dir, "llms.txt"))
	assert.NoFileExists(t, filepath.Join(dir, "llms-full.txt"))
}

func TestLLMsTxt_RootDirDotIsAccepted(t *testing.T) {
	dir := llmsProject(t, "[llms_txt]\ndir = \".\"\n")
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())
	assert.Equal(t, ".", cfg.LLMsTxtDir())
}
