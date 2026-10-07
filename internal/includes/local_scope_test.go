package includes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestResolver_CreateSource_LocalIncludeStaysInsideProject(t *testing.T) {
	// Arrange: <root>/project (the project), <root>/victim (outside), a link
	// inside the project that points at the victim.
	root := t.TempDir()
	project := filepath.Join(root, "project")
	victim := filepath.Join(root, "victim")
	require.NoError(t, os.MkdirAll(filepath.Join(project, "shared"), 0o755))
	require.NoError(t, os.MkdirAll(victim, 0o755))
	testutil.SymlinkOrSkip(t, victim, filepath.Join(project, "link"))
	overlay := &config.LocalOverlay{Doc: map[string]any{"includes": []any{map[string]any{"name": "x", "source": "../victim"}}}}

	tests := []struct {
		name    string
		source  string
		cfg     *config.Config
		wantErr bool
	}{
		{"relative inside", "shared", &config.Config{}, false},
		{"dot-dot outside", "../victim", &config.Config{}, true},
		{"absolute outside", victim, &config.Config{}, true},
		{"symlink inside pointing outside", "link", &config.Config{}, true},
		{"absolute inside", filepath.Join(project, "shared"), &config.Config{}, false},
		{"outside allowed from the local overlay", "../victim", &config.Config{LocalOverlay: overlay}, false},
		{"outside allowed in user scope", "../victim", &config.Config{UserScope: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			r := &Resolver{baseDir: project, cfg: tt.cfg}

			// Act
			_, err := r.createSource(t.Context(), &config.IncludeConfig{Name: "x", Source: tt.source})

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "outside the project")
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestResolver_CreateSource_CommittedLocalOverrideStaysInsideProject(t *testing.T) {
	// Arrange
	root := t.TempDir()
	project := filepath.Join(root, "project")
	require.NoError(t, os.MkdirAll(project, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "victim"), 0o755))
	r := &Resolver{baseDir: project, cfg: &config.Config{}}

	// Act
	_, err := r.createSource(t.Context(), &config.IncludeConfig{Name: "x", Source: "https://example.com/r.git", LocalOverride: "../victim"})

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside the project")
}

// A local include that leaves the project is an error when the project config
// declares it, not a warning that drops the include and exits 0; the machine-local
// overlay stays allowed.
func TestResolveIncludes_OutsideLocalIncludeIsAnError(t *testing.T) {
	tests := []struct {
		name    string
		overlay bool
		wantErr bool
	}{
		{"declared in the committed config", false, true},
		{"declared in the local overlay", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			project := filepath.Join(root, "project")
			require.NoError(t, os.MkdirAll(filepath.Join(project, ".ai-rulez"), 0o755))
			require.NoError(t, os.MkdirAll(filepath.Join(root, "victim", "rules"), 0o755))
			cfg := &config.Config{
				BaseDir: project, ConfigDir: filepath.Join(project, ".ai-rulez"),
				Includes: []config.IncludeConfig{{Name: "x", Source: "../victim"}},
				Content:  &config.ContentTree{Domains: map[string]*config.Domain{}},
			}
			if tt.overlay {
				cfg.LocalOverlay = &config.LocalOverlay{Doc: map[string]any{"includes": []any{map[string]any{"name": "x", "source": "../victim"}}}}
			}

			// Act
			_, err := NewResolver(project, "").ResolveIncludes(t.Context(), cfg)

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, config.ErrIncludeOutsideProject)
				assert.Contains(t, err.Error(), "outside the project")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestResolver_CreateSource_FileURLStaysInsideProject(t *testing.T) {
	t.Setenv(lockfile.EnvAllowFileURLs, "")
	// Arrange
	root := t.TempDir()
	project := filepath.Join(root, "project")
	victim := filepath.Join(root, "victim")
	require.NoError(t, os.MkdirAll(filepath.Join(project, "shared"), 0o755))
	require.NoError(t, os.MkdirAll(victim, 0o755))
	overlay := &config.LocalOverlay{Doc: map[string]any{"includes": []any{map[string]any{"name": "x", "source": "file://" + victim}}}}

	tests := []struct {
		name    string
		source  string
		cfg     *config.Config
		wantErr bool
	}{
		{"file url outside", "file://" + victim, &config.Config{}, true},
		{"file url dot-dot outside", "file://" + filepath.Join(project, "..", "victim"), &config.Config{}, true},
		{"file url inside", "file://" + filepath.Join(project, "shared"), &config.Config{}, false},
		{"file url outside from the local overlay", "file://" + victim, &config.Config{LocalOverlay: overlay}, false},
		{"file url outside in user scope", "file://" + victim, &config.Config{UserScope: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			r := &Resolver{baseDir: project, cfg: tt.cfg}

			// Act
			_, err := r.createSource(t.Context(), &config.IncludeConfig{Name: "x", Source: tt.source})

			// Assert
			if tt.wantErr {
				require.ErrorIs(t, err, config.ErrIncludeOutsideProject)
				return
			}
			if err != nil {
				assert.NotContains(t, err.Error(), "outside the project")
			}
		})
	}
}
