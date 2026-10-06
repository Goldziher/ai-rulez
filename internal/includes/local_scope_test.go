package includes

import (
	"os"
	"path/filepath"
	"testing"

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
	if err := os.Symlink(victim, filepath.Join(project, "link")); err != nil {
		t.Skip("symlinks unavailable")
	}
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
