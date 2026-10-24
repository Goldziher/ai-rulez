package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRecursiveExclude(t *testing.T) {
	write := func(t *testing.T, rel, body string) string {
		dir := t.TempDir()
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		return dir
	}
	const withKeys = "version = \"5.0\"\nrecursive_exclude = [\"a\", \"b/*\"]\n"

	cases := []struct {
		name      string
		dir       func(t *testing.T) string
		configDir string
		want      []string
		wantErr   bool
	}{
		{"ai-rulez layout", func(t *testing.T) string { return write(t, ".ai-rulez/config.toml", withKeys) }, "", []string{"a", "b/*"}, false},
		{"agents layout", func(t *testing.T) string { return write(t, ".agents/ai-rulez.toml", withKeys) }, "", []string{"a", "b/*"}, false},
		{".config layout", func(t *testing.T) string { return write(t, ".config/ai-rulez/config.toml", withKeys) }, "", []string{"a", "b/*"}, false},
		{"explicit config dir", func(t *testing.T) string { return write(t, "custom/config.toml", withKeys) }, "custom", []string{"a", "b/*"}, false},
		{"no key", func(t *testing.T) string { return write(t, ".ai-rulez/config.toml", "version = \"5.0\"\n") }, "", nil, false},
		{"no configuration", func(t *testing.T) string { return t.TempDir() }, "", nil, false},
		{"configuration only above dir", func(t *testing.T) string {
			root := write(t, ".ai-rulez/config.toml", withKeys)
			sub := filepath.Join(root, "sub")
			require.NoError(t, os.MkdirAll(sub, 0o755))
			return sub
		}, "", nil, false},
		{"invalid TOML", func(t *testing.T) string { return write(t, ".ai-rulez/config.toml", "recursive_exclude = [") }, "", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := LoadRecursiveExclude(tc.dir(t), tc.configDir)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestExcludedFromRecursion(t *testing.T) {
	patterns := []string{"tests/fixtures/", "svc/*/gen", "./docs/gen"}
	cases := []struct {
		rel  string
		want bool
	}{
		{"tests/fixtures", true},
		{"tests/fixtures/config/basic", true},
		{"tests/other", false},
		{"tests/fixtures2", false},
		{"svc/api/gen", true},
		{"svc/api/gen/deep", true},
		{"svc/api", false},
		{"docs/gen/x", true},
		{"docs", false},
		{".", false},
	}
	for _, tc := range cases {
		t.Run(tc.rel, func(t *testing.T) {
			assert.Equal(t, tc.want, ExcludedFromRecursion(tc.rel, patterns))
		})
	}
	assert.False(t, ExcludedFromRecursion("tests/fixtures", nil), "no patterns exclude nothing")
}
