package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
)

func requireValidBundle(t *testing.T, configDir string) {
	t.Helper()
	b, err := okf.Load(os.DirFS(configDir))
	require.NoError(t, err)
	for _, f := range append(b.CheckRoot(), b.Validate()...) {
		assert.Contains(t, f.Message, "subdirectory", "unexpected OKF finding: %+v", f)
	}
}

func TestInitScaffoldsAnOKFBundle(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	setForce(t, false, false)
	skipContentFlag, domainsFlag, configDir = false, "", ""

	runInit(InitCmd, []string{"demo"})

	configDir := filepath.Join(dir, ".ai-rulez")
	root, err := os.ReadFile(filepath.Join(configDir, "index.md"))
	require.NoError(t, err)
	assert.Contains(t, string(root), "okf_version")
	rule, err := os.ReadFile(filepath.Join(configDir, "rules", "code-quality.md"))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(rule), "---\ntype: Decision\ntitle: Code Quality\n"), string(rule))
	assert.Contains(t, string(rule), "x-ai-rulez:")
	assert.FileExists(t, filepath.Join(configDir, "skills", "code-reviewer", "SKILL.md"))
	assert.FileExists(t, filepath.Join(configDir, "skills", "index.md"))
	requireValidBundle(t, configDir)

	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	require.Len(t, cfg.Content.Rules, 1)
	assert.Equal(t, "high", cfg.Content.Rules[0].Metadata.Priority)
}

func TestInitSkipContentStillWritesTheRootIndex(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	setForce(t, false, false)
	skipContentFlag, domainsFlag, configDir = true, "", ""
	t.Cleanup(func() { skipContentFlag = false })

	runInit(InitCmd, []string{"demo"})

	root, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "index.md"))
	require.NoError(t, err)
	assert.Contains(t, string(root), "okf_version")
}

func TestInitFromImportWritesAnOKFBundle(t *testing.T) {
	dir := initFromProject(t)
	chdir(t, dir)
	setInitFlags(t, "auto")

	runInit(InitCmd, nil)

	configDir := filepath.Join(dir, ".ai-rulez")
	assert.FileExists(t, filepath.Join(configDir, "index.md"))
	rule, err := os.ReadFile(filepath.Join(configDir, "rules", "ts.md"))
	require.NoError(t, err)
	assert.Contains(t, string(rule), "type: Decision")
}
