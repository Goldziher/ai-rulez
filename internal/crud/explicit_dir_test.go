package crud_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
)

func writeProject(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"),
		[]byte("version = \"5.0\"\nname = \"explicit\"\npresets = [\"claude\"]\n"), 0o644))
}

// NewOperatorAt acts on exactly the config directory it is given, so two
// projects (or a differently named directory) never get mixed up by discovery.
func TestNewOperatorAt_ActsOnTheGivenConfigDir(t *testing.T) {
	// Arrange: a discoverable .ai-rulez/ and a second, differently named config dir
	base := t.TempDir()
	writeProject(t, filepath.Join(base, ".ai-rulez"))
	other := filepath.Join(base, "sub", "team-rules")
	writeProject(t, other)
	ctx := context.Background()

	// Act
	op, err := crud.NewOperatorAt(other)
	require.NoError(t, err)
	_, err = op.AddDomain(ctx, &crud.AddDomainRequest{Name: "backend"})
	require.NoError(t, err)
	require.NoError(t, op.AddProfile(ctx, "api", []string{"backend"}))
	profiles, err := op.ListProfiles(ctx)

	// Assert
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, other, op.ConfigDir())
	assert.DirExists(t, filepath.Join(other, "domains", "backend"))
	assert.NoDirExists(t, filepath.Join(base, ".ai-rulez", "domains", "backend"))
}

func TestNewOperatorAt_MissingDirIsAnError(t *testing.T) {
	_, err := crud.NewOperatorAt(filepath.Join(t.TempDir(), "nope"))
	require.Error(t, err)
}

func TestNewOperatorAt_LocalOverlayLandsInTheGivenDir(t *testing.T) {
	base := t.TempDir()
	writeProject(t, filepath.Join(base, ".ai-rulez"))
	other := filepath.Join(base, "team-rules")
	writeProject(t, other)
	ctx := context.Background()

	op, err := crud.NewOperatorAt(other)
	require.NoError(t, err)
	local := op.Local()
	_, err = local.AddDomain(ctx, &crud.AddDomainRequest{Name: "scratch"})
	require.NoError(t, err)
	require.NoError(t, local.AddProfile(ctx, "mine", []string{"scratch"}))

	entries, err := filepath.Glob(filepath.Join(other, "config.local.*"))
	require.NoError(t, err)
	assert.NotEmpty(t, entries, "the overlay is written next to the chosen config")
	none, _ := filepath.Glob(filepath.Join(base, ".ai-rulez", "config.local.*"))
	assert.Empty(t, none)
}
