package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func okfProject(t *testing.T, cfgExtra string) (string, *Generator) {
	t.Helper()
	dir := t.TempDir()
	put := func(rel, content string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	put(".ai-rulez/config.yaml", "version: \"4.0\"\nname: okf-test\npresets:\n  - claude\n  - okf\ngitignore: true\n"+cfgExtra)
	put(".ai-rulez/rules/style.md", "---\ndescription: Code style\npriority: high\n---\nUse gofmt.\n")
	put(".ai-rulez/rules/extra.md", "---\ndescription: Extra\n---\nTemp.\n")
	put(".ai-rulez/context/overview.md", "---\ndescription: Overview\n---\nWhat this is.\n")
	put(".ai-rulez/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Deploy it\n---\nSteps.\n")
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	return dir, NewGenerator(cfg)
}

func TestOKFPresetWritesConformantBundleAndTracksDrift(t *testing.T) {
	dir, gen := okfProject(t, "")
	require.NoError(t, gen.Generate("default"))

	bundle := filepath.Join(dir, "docs", "okf")
	b, err := okf.Load(os.DirFS(bundle))
	require.NoError(t, err)
	assert.Empty(t, b.Validate())
	assert.Equal(t, "Decision", b.Concepts["rules/style.md"].Type())
	raw, err := os.ReadFile(filepath.Join(bundle, "rules", "style.md"))
	require.NoError(t, err)
	assert.True(t, len(raw) > 4 && string(raw[:4]) == "---\n", "no banner before the frontmatter")

	drift, err := gen.CheckDrift("default")
	require.NoError(t, err)
	assert.Empty(t, drift)

	gitignore, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	assert.NotContains(t, string(gitignore), "docs/okf", "the bundle is committed documentation")

	// A hand edit of the bundle is drift.
	require.NoError(t, os.WriteFile(filepath.Join(bundle, "rules", "style.md"), []byte("edited\n"), 0o644))
	drift, err = gen.CheckDrift("default")
	require.NoError(t, err)
	require.Len(t, drift, 1)
	assert.Equal(t, "docs/okf/rules/style.md", drift[0].Path)
	require.NoError(t, gen.Generate("default"))

	// Removing a source removes its concept and its index entry.
	require.NoError(t, os.Remove(filepath.Join(dir, ".ai-rulez", "rules", "extra.md")))
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	gen = NewGenerator(cfg)
	require.NoError(t, gen.Generate("default"))
	_, statErr := os.Stat(filepath.Join(bundle, "rules", "extra.md"))
	assert.True(t, os.IsNotExist(statErr))
	b, err = okf.Load(os.DirFS(bundle))
	require.NoError(t, err)
	assert.Empty(t, b.Validate())
}

func TestOKFPresetHonorsDirAndInclude(t *testing.T) {
	dir, gen := okfProject(t, "okf:\n  dir: knowledge/bundle\n  include: [rules]\n")
	require.NoError(t, gen.Generate("default"))
	b, err := okf.Load(os.DirFS(filepath.Join(dir, "knowledge", "bundle")))
	require.NoError(t, err)
	assert.Empty(t, b.Validate())
	assert.Contains(t, b.Concepts, "rules/style.md")
	assert.NotContains(t, b.Concepts, "context/overview.md")
	_, statErr := os.Stat(filepath.Join(dir, "docs", "okf"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestOKFConfigValidation(t *testing.T) {
	for _, bad := range []string{"okf:\n  spec: \"0.3\"\n", "okf:\n  dir: ../out\n", "okf:\n  dir: .ai-rulez/okf\n", "okf:\n  include: [things]\n"} {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.yaml"), []byte("version: \"4.0\"\nname: x\npresets: [okf]\n"+bad), 0o644))
		cfg, err := config.LoadConfig(context.Background(), dir)
		if err == nil {
			err = cfg.Validate()
		}
		assert.Error(t, err, bad)
	}
}
