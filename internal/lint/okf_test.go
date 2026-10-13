package lint

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/okf"
)

func TestOKFCodesMatchPackage(t *testing.T) {
	assert.Equal(t, okf.CodeIndexMismatch, CodeOKFIndexMismatch)
	assert.Equal(t, okf.CodeTypeInvalid, CodeOKFTypeInvalid)
	assert.Equal(t, okf.CodeLinkBroken, CodeOKFLinkBroken)
	assert.Equal(t, okf.CodeVersionInvalid, CodeOKFVersionInvalid)
	assert.Equal(t, okf.CodeOrphan, CodeOKFOrphan)
	assert.Equal(t, okf.CodeExportDrift, CodeOKFExportDrift)
	assert.Equal(t, okf.CodeReservedStructure, CodeOKFReservedStructure)
	assert.Equal(t, okf.CodeTitleDuplicate, CodeOKFTitleDuplicate)
	assert.Equal(t, okf.CodePathUnsafe, CodeOKFPathUnsafe)
	assert.Equal(t, okf.CodeLossyMapping, CodeOKFLossyMapping)
}

func TestWithOKFAccumulatesBundles(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".ai-rulez/config.toml": baseConfig})
	gitAdd(t, root)
	cfg, err := loadWithResolvers(context.Background(), root)
	require.NoError(t, err)
	tree, err := LoadTree(root)
	require.NoError(t, err)
	a := []okf.Finding{okf.NewFinding(okf.CodeTypeInvalid, "a.md", 1, "first")}
	b := []okf.Finding{okf.NewFinding(okf.CodeTypeInvalid, "b.md", 1, "second")}

	rep, err := RunWith(cfg, tree, Options{}, WithOKF(filepath.Join(root, "dirA"), a), WithOKF(filepath.Join(root, "dirB"), b))

	require.NoError(t, err)
	assert.True(t, has(rep.Findings, CodeOKFTypeInvalid, "dirA/a.md", 1), dump(rep.Findings))
	assert.True(t, has(rep.Findings, CodeOKFTypeInvalid, "dirB/b.md", 1), dump(rep.Findings))
}

func TestGlobNoMatchReadsXAIRulezMetadataPaths(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml": baseConfig,
		".ai-rulez/index.md":    "---\nokf_version: \"0.2\"\n---\n# Concepts\n",
		".ai-rulez/rules/g.md": "---\ntype: Decision\ntitle: G\nx-ai-rulez:\n  kind: rule\n  id: g\n  metadata:\n" +
			"    paths: [\"nomatch/**\", \"src/**\"]\n    activation: glob\n---\nbody\n",
		"src/a.py": "x = 1\n",
	})
	gitAdd(t, root)

	fs := lintDir(t, root)

	assert.Equal(t, 1, countCode(fs, CodeGlobNoMatch), dump(fs))
	assert.True(t, has(fs, CodeGlobNoMatch, "rules/g.md", 0), dump(fs))
}
