package migrate_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/migrate"
)

// Each directory under testdata holds in/ (a 4.x project), want/ (the expected
// 5.0 project) and optionally options.txt with one option per line.
// Regenerate want/ with UPDATE_GOLDEN=1 and review the diff.

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	require.NoError(t, filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		rel, _ := filepath.Rel(from, path)
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		return os.WriteFile(dst, data, 0o644)
	}))
}

func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	}))
	return out
}

func optionsFor(t *testing.T, dir string, root string) migrate.Options {
	t.Helper()
	opts := migrate.Options{Root: root, Recursive: true}
	data, err := os.ReadFile(filepath.Join(dir, "options.txt"))
	if os.IsNotExist(err) {
		return opts
	}
	require.NoError(t, err)
	for _, line := range strings.Fields(string(data)) {
		switch line {
		case "adopt-defaults":
			opts.AdoptDefaults = true
		case "write":
			opts.Write = true
		}
	}
	return opts
}

func TestMigrateGolden(t *testing.T) {
	cases, err := filepath.Glob(filepath.Join("testdata", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, cases)
	for _, dir := range cases {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			work := t.TempDir()
			copyTree(t, filepath.Join(dir, "in"), work)
			opts := optionsFor(t, dir, work)

			report, err := migrate.Run(opts)
			require.NoError(t, err)
			require.False(t, report.Failed(), "%+v", report.Projects)

			got := readTree(t, work)
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				require.NoError(t, os.RemoveAll(filepath.Join(dir, "want")))
				for rel, body := range got {
					dst := filepath.Join(dir, "want", filepath.FromSlash(rel))
					require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))
					require.NoError(t, os.WriteFile(dst, []byte(body), 0o644))
				}
			}
			assert.Equal(t, readTree(t, filepath.Join(dir, "want")), got)

			// The result loads with the v5 loader and a second run has nothing to do.
			for _, p := range report.Projects {
				base := filepath.Join(work, filepath.FromSlash(p.Path))
				cfg, err := config.LoadConfig(context.Background(), base)
				require.NoError(t, err, p.Path)
				assert.Equal(t, config.ConfigVersionV5, cfg.Version)
			}
			again, err := migrate.Run(opts)
			require.NoError(t, err)
			assert.False(t, again.Pending(), "a migrated project must be a fixed point: %+v", again.Projects)
			assert.Equal(t, got, readTree(t, work))
		})
	}
}

func TestDryRunAndCheckWriteNothing(t *testing.T) {
	for _, mode := range []string{"dry-run", "check"} {
		t.Run(mode, func(t *testing.T) {
			work := t.TempDir()
			copyTree(t, filepath.Join("testdata", "yaml-full", "in"), work)
			before := readTree(t, work)

			report, err := migrate.Run(migrate.Options{Root: work, DryRun: mode == "dry-run", Check: mode == "check", Recursive: true})

			require.NoError(t, err)
			assert.True(t, report.Pending())
			assert.Equal(t, before, readTree(t, work))
			assert.Equal(t, migrate.SchemaVersion, report.SchemaVersion)
		})
	}
}

func TestOlderThanFourIsRefused(t *testing.T) {
	work := t.TempDir()
	dir := filepath.Join(work, ".ai-rulez")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("version: \"3.0\"\nname: x\npresets: [claude]\n"), 0o644))

	report, err := migrate.Run(migrate.Options{Root: work})

	require.NoError(t, err)
	require.True(t, report.Failed())
	assert.Contains(t, report.Projects[0].Error, "install ai-rulez 4.x")
	assert.FileExists(t, filepath.Join(dir, "config.yaml"))
	assert.NoFileExists(t, filepath.Join(dir, "config.toml"))
}

func TestNoConfigDirIsAnError(t *testing.T) {
	_, err := migrate.Run(migrate.Options{Root: t.TempDir()})
	require.Error(t, err)
}

func TestAlreadyV5IsUnchanged(t *testing.T) {
	work := t.TempDir()
	dir := filepath.Join(work, ".ai-rulez")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	body := "version = \"5.0\"\nname = \"x\"\npresets = [\"claude\"]\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o644))

	report, err := migrate.Run(migrate.Options{Root: work})

	require.NoError(t, err)
	assert.False(t, report.Pending())
	assert.Equal(t, migrate.StatusUnchanged, report.Projects[0].Status)
	got, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
}
