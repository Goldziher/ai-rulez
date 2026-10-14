package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// setTargetFlags gives validate --path and --glob the values of one run and
// restores them afterwards (flag state is process-wide).
func setTargetFlags(t *testing.T, paths, globs []string) {
	t.Helper()
	set := func(name string, values []string) {
		f := ValidateCmd.Flags().Lookup(name)
		require.NotNil(t, f, "validate has no --%s", name)
		slice, ok := f.Value.(pflag.SliceValue)
		require.True(t, ok)
		require.NoError(t, slice.Replace(values))
		f.Changed = len(values) > 0
	}
	set(flagLintPath, paths)
	set(flagLintGlob, globs)
	t.Cleanup(func() { set(flagLintPath, nil); set(flagLintGlob, nil) })
}

const oddMarketplaceJSON = `{"name":"acme","owner":{"name":"A"},"plugins":[{"name":"a","source":"plugins/a"}]}`

func targetProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		".ai-rulez/config.toml":     "version = \"5.0\"\nname = \"p\"\ngitignore = false\nagents_md = false\npresets = [\"claude\"]\n",
		"catalog/odd/registry.json": oddMarketplaceJSON,
	}
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	return root
}

func findingFiles(rep *lint.Report, code string) []string {
	var out []string
	for _, f := range rep.Findings {
		if f.Code == code {
			out = append(out, f.File)
		}
	}
	return out
}

func TestStrictLintHonorsPathAndGlobFlags(t *testing.T) {
	tests := []struct {
		name         string
		paths, globs []string
		want         int
	}{
		{name: "no flags", want: 0},
		{name: "--path directory", paths: []string{"catalog"}, want: 1},
		{name: "--path file", paths: []string{"catalog/odd/registry.json"}, want: 1},
		{name: "--glob", globs: []string{"catalog/**/*.json"}, want: 1},
		{name: "--path and --glob name one file", paths: []string{"catalog"}, globs: []string{"**/registry.json"}, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := targetProject(t)
			t.Chdir(root)
			setTargetFlags(t, tt.paths, tt.globs)
			cfg, err := config.LoadConfig(t.Context(), root, config.WithoutRemote())
			require.NoError(t, err)

			rep, err := strictLint(t.Context(), cfg)

			require.NoError(t, err)
			assert.Len(t, findingFiles(rep, lint.CodePluginManifest), tt.want)
		})
	}
}

func TestStrictLintHonorsLintPathsConfig(t *testing.T) {
	root := targetProject(t)
	cfgPath := filepath.Join(root, ".ai-rulez", "config.toml")
	body, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfgPath, append(body, []byte("\n[lint]\npaths = [\"catalog\"]\n")...), 0o644))
	t.Chdir(root)
	cfg, err := config.LoadConfig(t.Context(), root, config.WithoutRemote())
	require.NoError(t, err)

	rep, err := strictLint(t.Context(), cfg)

	require.NoError(t, err)
	assert.Len(t, findingFiles(rep, lint.CodePluginManifest), 1)
}

func TestStrictLintRefusesAPathOutsideTheProject(t *testing.T) {
	root := targetProject(t)
	t.Chdir(root)
	setTargetFlags(t, []string{".."}, nil)
	cfg, err := config.LoadConfig(t.Context(), root, config.WithoutRemote())
	require.NoError(t, err)

	_, err = strictLint(t.Context(), cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside the project root")
}

func TestTargetFlagsAreStrictOnly(t *testing.T) {
	t.Cleanup(func() { validateStrict, validateConfigOnly = false, false })
	setTargetFlags(t, []string{"x"}, nil)
	validateStrict = false
	require.Error(t, checkStrictFlags(), "--path with --config-only must be rejected")
	validateStrict = true
	require.NoError(t, checkStrictFlags())

	t.Cleanup(func() { validateUpdateBaseline = false })
	validateUpdateBaseline = true
	require.Error(t, checkStrictFlags(), "--update-baseline needs every finding, not a selection")
}

func TestScanCommandHasTargetFlags(t *testing.T) {
	for _, name := range []string{flagLintPath, flagLintGlob} {
		assert.NotNil(t, ScanCmd.Flags().Lookup(name), "scan is missing --%s", name)
		assert.NotNil(t, ValidateCmd.Flags().Lookup(name), "validate is missing --%s", name)
	}
}
