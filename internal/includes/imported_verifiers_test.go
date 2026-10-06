package includes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func writeVerifierFile(t *testing.T, aiRulezDir, name, data string) {
	t.Helper()
	dir := filepath.Join(aiRulezDir, "verifiers")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644))
}

func TestResolveIncludes_CarriesVerifierFilesTaggedWithTheirInclude(t *testing.T) {
	// Arrange
	base := t.TempDir()
	one := createTestAIRulezDir(t, filepath.Join(base, "one"))
	two := createTestAIRulezDir(t, filepath.Join(base, "two"))
	writeVerifierFile(t, one, "a.toml", "# a\n")
	writeVerifierFile(t, one, "notes.txt", "not a declaration")
	writeVerifierFile(t, two, "b.toml", "# b\n")
	cfg := &config.Config{
		BaseDir: base,
		Includes: []config.IncludeConfig{
			{Name: "one", Source: filepath.Join(base, "one")},
			{Name: "two", Source: filepath.Join(base, "two"), InstallTo: "domains/extra"},
		},
		Content: &config.ContentTree{Domains: map[string]*config.Domain{}},
	}

	// Act
	tree, err := NewResolver(base, "").ResolveIncludes(context.Background(), cfg)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []config.ImportedVerifierFile{
		{Include: "one", Name: "a.toml", Data: "# a\n"},
		{Include: "two", Name: "b.toml", Data: "# b\n"},
	}, tree.ImportedVerifiers)
}

func TestResolveIncludes_VerifierSymlinkIsNeverFollowed(t *testing.T) {
	base := t.TempDir()
	dir := createTestAIRulezDir(t, filepath.Join(base, "one"))
	secret := filepath.Join(base, "secret.toml")
	require.NoError(t, os.WriteFile(secret, []byte("[[verifiers]]\nid = \"x\"\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "verifiers"), 0o755))
	testutil.SymlinkOrSkip(t, secret, filepath.Join(dir, "verifiers", "link.toml"))
	cfg := &config.Config{BaseDir: base, Includes: []config.IncludeConfig{{Name: "one", Source: filepath.Join(base, "one")}},
		Content: &config.ContentTree{Domains: map[string]*config.Domain{}}}

	tree, err := NewResolver(base, "").ResolveIncludes(context.Background(), cfg)

	require.NoError(t, err)
	assert.Empty(t, tree.ImportedVerifiers)
}
