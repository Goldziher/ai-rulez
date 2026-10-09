package cli

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okfNativeProject is a project whose content tree was migrated in place to an
// OKF bundle, so validate also runs the OKF checks over it.
func okfNativeProject(t *testing.T, env *isoEnv) string {
	t.Helper()
	root := minimalProject(t, "")
	res := env.run(root, "migrate", "okf")
	require.Equal(t, 0, res.ExitCode, res.Stdout+res.Stderr)
	return root
}

func TestValidateOKFProblemsJSONDocumentE2E(t *testing.T) {
	env := newIsoEnv(t)
	root := okfNativeProject(t, env)
	writeTree(t, root, map[string]string{".ai-rulez/rules/untyped.md": "---\ntitle: Untyped\n---\n\n# Untyped\n"})

	res := env.run(root, "validate", "--format", "json")

	assert.Equal(t, 2, res.ExitCode, res.Stdout+res.Stderr)
	doc := requireJSONDoc(t, res)
	findings, ok := doc["findings"].([]any)
	require.True(t, ok, res.Stdout)
	require.NotEmpty(t, findings)
	assert.Contains(t, res.Stdout, "AR9B1")
	assert.NotContains(t, res.Stdout+res.Stderr, "Configuration is valid")
}

func TestValidateOKFProblemsNeverSayValidE2E(t *testing.T) {
	env := newIsoEnv(t)
	root := okfNativeProject(t, env)
	writeTree(t, root, map[string]string{".ai-rulez/rules/untyped.md": "---\ntitle: Untyped\n---\n\n# Untyped\n"})

	res := env.run(root, "validate")

	assert.Equal(t, 2, res.ExitCode, res.Stdout+res.Stderr)
	assert.Contains(t, res.Stdout+res.Stderr, "AR9B1")
	assert.NotContains(t, res.Stdout+res.Stderr, "Configuration is valid")
}

// A rule without a frontmatter block is plain native markdown: generate accepts
// it, so validate reports it as a warning (exit 0) and generate --check is clean.
func TestFrontmatterlessRuleValidateAndGenerateAgreeE2E(t *testing.T) {
	env := newIsoEnv(t)
	root := okfNativeProject(t, env)
	writeTree(t, root, map[string]string{".ai-rulez/rules/plain.md": "# Plain\n\nNo frontmatter here.\n"})

	val := env.run(root, "validate", "--format", "json")
	assert.Equal(t, 0, val.ExitCode, val.Stdout+val.Stderr)
	gen := env.run(root, "generate")
	require.Equal(t, 0, gen.ExitCode, gen.Stdout+gen.Stderr)
	check := env.run(root, "generate", "--check")
	assert.Equal(t, 0, check.ExitCode, check.Stdout+check.Stderr)
	assert.FileExists(t, filepath.Join(root, "CLAUDE.md"))
}

func TestValidateQuietHidesSummariesE2E(t *testing.T) {
	env := newIsoEnv(t)
	root := okfNativeProject(t, env)

	res := env.run(root, "-q", "validate")

	assert.Equal(t, 0, res.ExitCode, res.Stdout+res.Stderr)
	out := res.Stdout + res.Stderr
	assert.NotContains(t, out, "strict validation:")
	assert.NotContains(t, out, "concepts,")
}
