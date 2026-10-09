package crud_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
)

// okfProject is a test project whose configuration directory is an OKF bundle.
func okfProject(t *testing.T) (*crud.OperatorImpl, string) {
	t.Helper()
	dir := setupTestProject(t)
	cfgDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, okfbridge.RefreshIndexes(t.Context(), cfgDir))
	op, err := crud.NewOperator(dir)
	require.NoError(t, err)
	return op, cfgDir
}

func TestAddWritesOKFConceptsAndIndexes(t *testing.T) {
	op, cfgDir := okfProject(t)
	ctx := context.Background()

	_, err := op.AddRule(ctx, &crud.AddFileRequest{Name: "go", Priority: "high", Targets: []string{"claude"}})
	require.NoError(t, err)
	_, err = op.AddSkill(ctx, &crud.AddFileRequest{Name: "review", Description: "Reviews code"})
	require.NoError(t, err)
	_, err = op.AddDomain(ctx, &crud.AddDomainRequest{Name: "web"})
	require.NoError(t, err)
	_, err = op.AddContext(ctx, &crud.AddFileRequest{Name: "ui", Domain: "web"})
	require.NoError(t, err)

	rule, err := os.ReadFile(filepath.Join(cfgDir, "rules", "go.md"))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(rule), "---\ntype: Decision\ntitle: Go\nx-ai-rulez:\n  kind: rule\n  id: go\n  metadata:\n    priority: high\n"), string(rule))

	idx, err := os.ReadFile(filepath.Join(cfgDir, "rules", "index.md"))
	require.NoError(t, err)
	assert.Contains(t, string(idx), "[Go](go.md)")
	web, err := os.ReadFile(filepath.Join(cfgDir, "domains", "web", "context", "index.md"))
	require.NoError(t, err)
	assert.Contains(t, string(web), "[Ui](ui.md)")

	b, err := okf.Load(os.DirFS(cfgDir))
	require.NoError(t, err)
	for _, f := range append(b.CheckRoot(), b.Validate()...) {
		assert.Contains(t, f.Message, "subdirectory", "unexpected finding: %+v", f)
	}

	// The loader reads the OKF files back.
	cfg, err := config.LoadConfig(ctx, filepath.Dir(cfgDir))
	require.NoError(t, err)
	require.Len(t, cfg.Content.Rules, 1)
	assert.Equal(t, "high", cfg.Content.Rules[0].Metadata.Priority)
	assert.Equal(t, []string{"claude"}, cfg.Content.Rules[0].Metadata.Targets)
	assert.Equal(t, "Decision", cfg.Content.Rules[0].Metadata.OKFType)
}

func TestRemoveRefreshesIndexes(t *testing.T) {
	op, cfgDir := okfProject(t)
	ctx := context.Background()
	_, err := op.AddRule(ctx, &crud.AddFileRequest{Name: "a"})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(cfgDir, "rules", "index.md"))

	require.NoError(t, op.RemoveFile(ctx, "", crud.ContentTypeRules, "a"))

	assert.NoFileExists(t, filepath.Join(cfgDir, "rules", "index.md"))
	root, err := os.ReadFile(filepath.Join(cfgDir, "index.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(root), "rules/")
}

func TestLegacyTreeGetsConceptsButNoIndexes(t *testing.T) {
	dir := setupTestProject(t)
	op, err := crud.NewOperator(dir)
	require.NoError(t, err)

	_, err = op.AddRule(context.Background(), &crud.AddFileRequest{Name: "go"})
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(dir, ".ai-rulez", "index.md"))
	rule, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "rules", "go.md"))
	require.NoError(t, err)
	assert.Contains(t, string(rule), "type: Decision")
}

func TestUpdateFileKeepsTypeAndTitle(t *testing.T) {
	op, cfgDir := okfProject(t)
	ctx := context.Background()
	custom := "---\ntype: Policy\ntitle: House Style\nx-ai-rulez:\n  kind: rule\n  id: style\n---\n\nBody\n"
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "rules", "style.md"), []byte(custom), 0o644))

	_, err := op.UpdateFile(ctx, "", crud.ContentTypeRules, "style", "New body\n", "high", nil)
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(cfgDir, "rules", "style.md"))
	require.NoError(t, err)
	assert.Contains(t, string(got), "type: Policy\ntitle: House Style\n")
	assert.Contains(t, string(got), "priority: high")
	assert.Contains(t, string(got), "New body")
}

func TestUpdateCheckEditsOKFFrontmatter(t *testing.T) {
	op, cfgDir := okfProject(t)
	ctx := context.Background()
	_, err := op.AddCheck(ctx, &crud.AddFileRequest{Name: "sec", Description: "Security"})
	require.NoError(t, err)

	_, err = op.UpdateCheck(ctx, "", "sec", "", false, crud.CheckFields{Severity: "high"})
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(cfgDir, "checks", "sec.md"))
	require.NoError(t, err)
	assert.Contains(t, string(got), "severity: high")
	assert.Equal(t, 1, strings.Count(string(got), "severity:"), "a second top-level severity would be shadowed:\n%s", got)
	assert.Contains(t, string(got), "x-ai-rulez:")
}

func TestAddRejectsBadNames(t *testing.T) {
	op, cfgDir := okfProject(t)
	ctx := context.Background()
	for _, name := range []string{"x.md", "X.MD", "a b", "tab\tname", "line\nbreak"} {
		_, err := op.AddRule(ctx, &crud.AddFileRequest{Name: name})
		assert.Error(t, err, "rule %q", name)
		_, err = op.AddContext(ctx, &crud.AddFileRequest{Name: name})
		assert.Error(t, err, "context %q", name)
		_, err = op.AddAgent(ctx, &crud.AddFileRequest{Name: name})
		assert.Error(t, err, "agent %q", name)
		_, err = op.AddCommand(ctx, &crud.AddFileRequest{Name: name})
		assert.Error(t, err, "command %q", name)
		_, err = op.AddCheck(ctx, &crud.AddFileRequest{Name: name})
		assert.Error(t, err, "check %q", name)
	}
	entries, err := os.ReadDir(filepath.Join(cfgDir, "rules"))
	require.NoError(t, err)
	assert.Empty(t, entries)
}
