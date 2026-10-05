package crud_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The project-level .config/ai-rulez/ layout must work for every CRUD
// operation, not only for generate/validate (#207).
func TestOperator_ConventionConfigDir(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	configDir := filepath.Join(baseDir, ".config", "ai-rulez")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"),
		[]byte("version = \"4.0\"\nname = \"convention\"\npresets = [\"claude\"]\n"), 0o644))
	ctx := context.Background()

	// Act
	op, err := crud.NewOperator(baseDir)
	require.NoError(t, err)
	_, err = op.AddDomain(ctx, &crud.AddDomainRequest{Name: "backend"})
	require.NoError(t, err)
	_, err = op.AddRule(ctx, &crud.AddFileRequest{Type: "rules", Name: "style", Content: "Be consistent."})
	require.NoError(t, err)
	require.NoError(t, op.AddProfile(ctx, "api", []string{"backend"}))

	// Assert
	profiles, err := op.ListProfiles(ctx)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, "api", profiles[0].Name)

	rules, err := op.ListFiles(ctx, "", "rules")
	require.NoError(t, err)
	require.Len(t, rules, 1)

	assert.DirExists(t, filepath.Join(configDir, "domains", "backend"))
	assert.FileExists(t, filepath.Join(configDir, "rules", "style.md"))
	assert.NoDirExists(t, filepath.Join(baseDir, ".ai-rulez"), "must not create a competing .ai-rulez/")
	assert.NoFileExists(t, filepath.Join(baseDir, ".config", "config.toml"))
}

func TestNewOperator_NoConfigDir(t *testing.T) {
	_, err := crud.NewOperator(t.TempDir())
	require.Error(t, err)
}
