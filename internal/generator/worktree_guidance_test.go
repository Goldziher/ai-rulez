package generator

import (
	"context"

	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// worktreeGuidanceProject writes a minimal project with the given presets and
// config extras.
func worktreeGuidanceProject(t *testing.T, root, presets, extra string) {
	t.Helper()
	cfgTOML := "version = \"5.0\"\nname = \"t\"\ngitignore = false\npresets = [" + presets + "]\nsession_start = { generate = false }\n" + extra
	path := filepath.Join(root, ".ai-rulez", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(cfgTOML), 0o644))
	rulesPath := filepath.Join(root, ".ai-rulez", "rules", "always.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(rulesPath), 0o755))
	require.NoError(t, os.WriteFile(rulesPath, []byte("# Always\n\nBody.\n"), 0o644))
}

func readGuidanceFile(t *testing.T, root, rel string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if os.IsNotExist(err) {
		return "", false
	}
	require.NoError(t, err)
	return string(data), true
}

func TestWorktreeGuidanceRendering(t *testing.T) {
	t.Run("the shared AGENTS.md carries it once and the shim does not repeat it", func(t *testing.T) {
		root := t.TempDir()
		worktreeGuidanceProject(t, root, `"codex", "opencode", "claude"`, "")
		cfg, err := config.LoadConfig(context.Background(), root)
		require.NoError(t, err)
		require.NoError(t, NewGenerator(cfg).Generate(""))

		agentsMD, ok := readGuidanceFile(t, root, "AGENTS.md")
		require.True(t, ok)
		assert.Equal(t, 1, strings.Count(agentsMD, "## Worktrees"), "one section in the shared file")
		assert.NotContains(t, agentsMD, "worktree_setup", "shared instructions work across MCP and CLI-only harnesses")
		claudeMD, ok := readGuidanceFile(t, root, "CLAUDE.md")
		require.True(t, ok)
		assert.NotContains(t, claudeMD, "## Worktrees", "the shim imports the shared file; the section must not repeat")

		// Several presets target the shared file; the section is identical
		// whichever writes it last, so a second run rewrites the same bytes.
		second := t.TempDir()
		worktreeGuidanceProject(t, second, `"opencode", "codex", "claude"`, "")
		cfg2, err := config.LoadConfig(context.Background(), second)
		require.NoError(t, err)
		require.NoError(t, NewGenerator(cfg2).Generate(""))
		other, ok := readGuidanceFile(t, second, "AGENTS.md")
		require.True(t, ok)
		assert.Equal(t, agentsMD, other, "the shared AGENTS.md is deterministic across preset orders")
	})

	t.Run("agents_md off puts the guidance in the full CLAUDE.md", func(t *testing.T) {
		root := t.TempDir()
		worktreeGuidanceProject(t, root, `"claude"`, "agents_md = false\n")
		cfg, err := config.LoadConfig(context.Background(), root)
		require.NoError(t, err)
		require.NoError(t, NewGenerator(cfg).Generate(""))

		claudeMD, ok := readGuidanceFile(t, root, "CLAUDE.md")
		require.True(t, ok)
		assert.Contains(t, claudeMD, "## Worktrees")
		_, ok = readGuidanceFile(t, root, "AGENTS.md")
		assert.False(t, ok, "agents_md off writes no shared AGENTS.md")
	})

	t.Run("MCP-capable configs prefer the tool and name the CLI fallback", func(t *testing.T) {
		root := t.TempDir()
		worktreeGuidanceProject(t, root, `"claude"`, "agents_md = false\n")
		cfg, err := config.LoadConfig(context.Background(), root)
		require.NoError(t, err)
		require.NoError(t, NewGenerator(cfg).Generate(""))
		claudeMD, ok := readGuidanceFile(t, root, "CLAUDE.md")
		require.True(t, ok)
		assert.Contains(t, claudeMD, "worktree_setup")
		assert.Contains(t, claudeMD, "the CLI command is the fallback")
	})

	t.Run("CLI-only configurations get the CLI-only wording", func(t *testing.T) {
		root := t.TempDir()
		worktreeGuidanceProject(t, root, `"claude"`, "agents_md = false\n[mcp]\nself_server = false\n")
		cfg, err := config.LoadConfig(context.Background(), root)
		require.NoError(t, err)
		require.NoError(t, NewGenerator(cfg).Generate(""))
		claudeMD, ok := readGuidanceFile(t, root, "CLAUDE.md")
		require.True(t, ok)
		assert.Contains(t, claudeMD, "ai-rulez worktree setup")
		assert.NotContains(t, claudeMD, "worktree_setup")
	})
}

func TestWorktreeGuidanceAgentsSource(t *testing.T) {
	root := t.TempDir()
	// An agents-source project: the authored AGENTS.md is the source, the
	// configuration lives in .agents/ai-rulez.toml.
	cfgTOML := "version = \"5.0\"\nname = \"t\"\ngitignore = false\npresets = [\"claude\"]\nsession_start = { generate = false }\n"
	path := filepath.Join(root, ".agents", "ai-rulez.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(cfgTOML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Authored instructions\n"), 0o644))
	rulesPath := filepath.Join(root, ".agents", "rules", "always.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(rulesPath), 0o755))
	require.NoError(t, os.WriteFile(rulesPath, []byte("# Always\n\nBody.\n"), 0o644))

	cfg, err := config.LoadConfig(context.Background(), root, config.WithResolvers(config.Resolvers{
		// A passthrough reader: this test pins the rendering decision, not
		// the agents-source reader itself.
		Agents: func(_ context.Context, _ *config.Config, tree *config.ContentTree) (*config.ContentTree, error) {
			return tree, nil
		},
	}))
	require.NoError(t, err)
	require.True(t, cfg.UsesAgentsSource())
	require.NoError(t, NewGenerator(cfg).Generate(""))

	agentsMD, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Authored instructions\n", string(agentsMD), "the authored source is preserved byte for byte")

	claudeMD, ok := readGuidanceFile(t, root, "CLAUDE.md")
	require.True(t, ok, "the shim is a generated file that is actually loaded")
	assert.Contains(t, claudeMD, "## Worktrees", "the guidance lands in the generated shim")
}
