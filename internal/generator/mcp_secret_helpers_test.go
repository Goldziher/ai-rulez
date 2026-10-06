package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/require"
)

const leakToken = "secret123-token-value"

// newSecretMCPRepo writes a git repo with one MCP server whose env references
// ${GITHUB_TOKEN}, one rule and one check, and returns its root.
func newSecretMCPRepo(t *testing.T, presets string) string {
	t.Helper()
	root := t.TempDir()
	initGitRepo(t, root)
	dir := filepath.Join(root, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "rules"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "checks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`version = "4.0"
name = "leaks"
presets = `+presets+`
gitignore = true

[[mcp_servers]]
name = "github"
command = "npx"
args = ["-y", "gh-mcp", "${PROJECT_ROOT}"]
env = { GITHUB_TOKEN = "${GITHUB_TOKEN}" }
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rules", "r.md"), []byte("# r\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "checks", "c.md"),
		[]byte("---\ndescription: d\nseverity: high\n---\n# c\nbody\n"), 0o644))
	t.Setenv("GITHUB_TOKEN", leakToken)
	return root
}

func generateRepo(t *testing.T, root string) *Generator {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	g := NewGenerator(cfg)
	require.NoError(t, g.Generate("default"))
	return g
}

func appendConfig(t *testing.T, root, extra string) {
	t.Helper()
	path := filepath.Join(root, ".ai-rulez", "config.toml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, extra...), 0o644))
}
