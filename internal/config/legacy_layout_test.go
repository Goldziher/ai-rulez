package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func layoutDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	return dir
}

func TestIsLegacyLayout(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"content without a root index", map[string]string{"config.toml": "", "rules/r.md": "# R\n"}, true},
		{"domain content without a root index", map[string]string{"domains/web/rules/r.md": "# R\n"}, true},
		{"an OKF bundle", map[string]string{"index.md": "---\nokf_version: \"0.2\"\n---\n", "rules/r.md": "# R\n"}, false},
		{"no content", map[string]string{"config.toml": ""}, false},
		{"hidden files are not content", map[string]string{"rules/.keep.md": ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsLegacyLayout(layoutDir(t, tt.files)))
		})
	}
}
