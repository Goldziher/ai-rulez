package okfbridge_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func codesOf(fs []okf.Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Code+" "+f.Path)
	}
	return out
}

func TestCheckProject(t *testing.T) {
	root := sampleProject(t)
	cfgFile := filepath.Join(root, ".ai-rulez", "config.yaml")
	require.NoError(t, os.WriteFile(cfgFile, []byte("version: \"4.0\"\nname: sample\npresets:\n  - claude\n  - okf\n"), 0o644))
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	tree := cfg.Content

	// Preset on but nothing generated yet.
	res, err := okfbridge.CheckProject(cfg, tree)
	require.NoError(t, err)
	assert.Equal(t, []string{"AR9B5 "}, codesOf(res.Findings))

	exp, err := okfbridge.Export(tree, okfbridge.ExportOptions{})
	require.NoError(t, err)
	require.NoError(t, okf.WriteFiles(filepath.Join(root, "docs", "okf"), exp.Files, true))
	res, err = okfbridge.CheckProject(cfg, tree)
	require.NoError(t, err)
	assert.Empty(t, res.Findings)

	// Edit, add a stray file, delete another.
	bundle := filepath.Join(root, "docs", "okf")
	require.NoError(t, os.WriteFile(filepath.Join(bundle, "rules", "plain.md"), []byte("---\ntype: Decision\n---\nchanged\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(bundle, "rules", "stray.md"), []byte("---\ntype: Decision\n---\n"), 0o644))
	require.NoError(t, os.Remove(filepath.Join(bundle, "context", "architecture.md")))
	res, err = okfbridge.CheckProject(cfg, tree)
	require.NoError(t, err)
	got := codesOf(res.Findings)
	assert.Contains(t, got, "AR9B5 rules/plain.md")
	assert.Contains(t, got, "AR9B5 rules/stray.md")
	assert.Contains(t, got, "AR9B5 context/architecture.md")
	assert.Contains(t, got, "AR9B0 context/index.md", "the index still lists the deleted file")

	// Without the preset the bundle is only linted, never compared.
	cfg.Presets = cfg.Presets[:1]
	cfg.OKF = &config.OKFConfig{Dir: "docs/okf"}
	res, err = okfbridge.CheckProject(cfg, tree)
	require.NoError(t, err)
	for _, f := range res.Findings {
		assert.NotEqual(t, okf.CodeExportDrift, f.Code)
	}

	cfg.OKF = nil
	res, err = okfbridge.CheckProject(cfg, tree)
	require.NoError(t, err)
	assert.Nil(t, res, "nothing configured, nothing checked")
}
