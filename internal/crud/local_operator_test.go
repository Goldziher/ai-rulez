package crud_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/crud"
)

// localOpProject is a project whose include and skill sources are local
// directories, so loading never touches the network.
type localOpProject struct {
	baseDir    string
	ext        string // directory holding the include and skill sources
	sharedPath string
	shared     string
	localPath  string
}

func setupLocalOpProject(t *testing.T) *localOpProject {
	t.Helper()
	p := &localOpProject{baseDir: t.TempDir()}
	p.ext = filepath.Join(p.baseDir, "ext")
	for _, d := range []string{"inc", "sk", "loc", "new"} {
		require.NoError(t, os.MkdirAll(filepath.Join(p.ext, d, ".ai-rulez"), 0o755))
	}
	dir := filepath.Join(p.baseDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "domains", "backend", "rules"), 0o755))
	p.shared = fmt.Sprintf(`version = "5.0"
name = "test-project"
presets = ["claude"]

[profiles]
base = ["backend"]

[[includes]]
name = "inc"
source = %q

[[installed_skills]]
name = "sk"
source = %q
`, filepath.ToSlash(filepath.Join(p.ext, "inc")), filepath.ToSlash(filepath.Join(p.ext, "sk")))
	p.sharedPath = filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(p.sharedPath, []byte(p.shared), 0o600))
	p.localPath = filepath.Join(dir, "config.local.toml")
	return p
}

func TestLocalOperator_RoutesConfigMutationsToOverlay(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		run   func(op *crud.OperatorImpl, ext string) error
		check func(t *testing.T, cfg *config.Config)
	}{
		{
			"AddProfile",
			func(op *crud.OperatorImpl, _ string) error { return op.AddProfile(ctx, "mine", []string{"backend"}) },
			func(t *testing.T, cfg *config.Config) { assert.Contains(t, cfg.Profiles, "mine") },
		},
		{
			"SetDefaultProfile",
			func(op *crud.OperatorImpl, _ string) error { return op.SetDefaultProfile(ctx, "base") },
			func(t *testing.T, cfg *config.Config) { assert.Equal(t, "base", cfg.Default) },
		},
		{
			"AddInclude",
			func(op *crud.OperatorImpl, ext string) error {
				return op.AddInclude(ctx, &crud.AddIncludeRequest{Name: "loc", Source: filepath.Join(ext, "loc")})
			},
			func(t *testing.T, cfg *config.Config) { assert.Len(t, cfg.Includes, 2) },
		},
		{
			"RemoveInclude of a shared include writes a remove marker",
			func(op *crud.OperatorImpl, _ string) error { return op.RemoveInclude(ctx, "inc") },
			func(t *testing.T, cfg *config.Config) { assert.Empty(t, cfg.Includes) },
		},
		{
			"InstallSkill",
			func(op *crud.OperatorImpl, ext string) error {
				return op.InstallSkill(ctx, &crud.InstallSkillRequest{Name: "new", Source: filepath.Join(ext, "new")})
			},
			func(t *testing.T, cfg *config.Config) { assert.Len(t, cfg.InstalledSkills, 2) },
		},
		{
			"UninstallSkill of a shared skill writes a remove marker",
			func(op *crud.OperatorImpl, _ string) error { return op.UninstallSkill(ctx, "sk") },
			func(t *testing.T, cfg *config.Config) { assert.Empty(t, cfg.InstalledSkills) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := setupLocalOpProject(t)
			op, err := crud.NewOperator(p.baseDir)
			require.NoError(t, err)

			// Act
			err = tt.run(op.Local(), p.ext)

			// Assert
			require.NoError(t, err)
			shared, err := os.ReadFile(p.sharedPath)
			require.NoError(t, err)
			assert.Equal(t, p.shared, string(shared), "the shared config must be untouched")
			info, err := os.Stat(p.localPath)
			require.NoError(t, err)
			if runtime.GOOS != "windows" { // Windows has no Unix permission bits
				assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
			}
			merged, err := loadWithResolvers(ctx, p.baseDir)
			require.NoError(t, err)
			tt.check(t, merged)
		})
	}
}

func TestLocalOperator_RemoveProfile(t *testing.T) {
	ctx := context.Background()

	t.Run("removes a local profile", func(t *testing.T) {
		p := setupLocalOpProject(t)
		op, err := crud.NewOperator(p.baseDir)
		require.NoError(t, err)
		require.NoError(t, op.Local().AddProfile(ctx, "mine", []string{"backend"}))

		require.NoError(t, op.Local().RemoveProfile(ctx, "mine"))

		merged, err := loadWithResolvers(ctx, p.baseDir)
		require.NoError(t, err)
		assert.NotContains(t, merged.Profiles, "mine")
	})

	t.Run("refuses to remove a shared profile locally", func(t *testing.T) {
		p := setupLocalOpProject(t)
		op, err := crud.NewOperator(p.baseDir)
		require.NoError(t, err)

		err = op.Local().RemoveProfile(ctx, "base")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "shared config")
	})
}

func TestLocalOperator_RejectsDuplicatesAcrossLayers(t *testing.T) {
	ctx := context.Background()
	p := setupLocalOpProject(t)
	op, err := crud.NewOperator(p.baseDir)
	require.NoError(t, err)

	assert.Error(t, op.Local().AddProfile(ctx, "base", []string{"backend"}))
	assert.Error(t, op.Local().AddInclude(ctx, &crud.AddIncludeRequest{Name: "inc", Source: filepath.Join(p.ext, "loc")}))
	assert.Error(t, op.Local().RemoveInclude(ctx, "ghost"))
}

func TestLocalOperator_AddProfileAcceptsLocalDomain(t *testing.T) {
	ctx := context.Background()
	p := setupLocalOpProject(t)
	scratch := filepath.Join(p.baseDir, ".ai-rulez", "local", "domains", "scratch", "rules")
	require.NoError(t, os.MkdirAll(scratch, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(scratch, "r.md"), []byte("# R\n"), 0o600))
	op, err := crud.NewOperator(p.baseDir)
	require.NoError(t, err)

	require.NoError(t, op.Local().AddProfile(ctx, "mine", []string{"scratch"}))

	// The shared path must still reject a domain teammates do not have.
	err = op.AddProfile(ctx, "shared-mine", []string{"scratch"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}
