package includes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func writeLocalRule(t *testing.T, dir, name string) {
	t.Helper()
	rules := filepath.Join(dir, "rules")
	require.NoError(t, os.MkdirAll(rules, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rules, name+".md"), []byte("# "+name+"\n\nbody\n"), 0o644))
}

func TestResolver_LocalIncludeHonorsPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantRule string
		wantErr  string
	}{
		{name: "no path loads the source", path: "", wantRule: "root"},
		{name: "path loads the subdirectory", path: "modules/x", wantRule: "mod"},
		{name: "missing path fails", path: "modules/nope", wantErr: "modules/nope"},
		{name: "path escaping the source fails", path: "../x", wantErr: "include"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base := t.TempDir()
			writeLocalRule(t, filepath.Join(base, "src"), "root")
			writeLocalRule(t, filepath.Join(base, "src", "modules", "x"), "mod")
			r := &Resolver{baseDir: base, cfg: &config.Config{}}

			// Act
			src, err := r.createSource(context.Background(), &config.IncludeConfig{Name: "x", Source: "./src", Path: tt.path})
			var tree *config.ContentTree
			if err == nil {
				tree, err = src.Fetch(context.Background())
			}

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, tree.Rules, 1)
			assert.Equal(t, tt.wantRule, tree.Rules[0].Name)
		})
	}
}
