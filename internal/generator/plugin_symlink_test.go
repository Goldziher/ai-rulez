package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// linkOutput turns an output of the generated bundle into a symlink to a file
// or directory inside the project and returns the link-relative target path.
type linkCase struct {
	name     string
	link     string // project-relative path that becomes a symlink
	wantLink string // the path the error and the drift name
	setup    func(t *testing.T, dir string) (target string)
}

func linkCases() []linkCase {
	return []linkCase{
		{
			name:     "file symlink",
			link:     "skills/core-s/SKILL.md",
			wantLink: "skills/core-s/SKILL.md",
			setup: func(t *testing.T, dir string) string {
				target := filepath.Join(dir, "README.md")
				writeDomainsFile(t, target, "readme\n")
				return target
			},
		},
		{
			name:     "parent directory symlink",
			link:     ".cursor-plugin",
			wantLink: ".cursor-plugin",
			setup: func(t *testing.T, dir string) string {
				target := filepath.Join(dir, "elsewhere")
				require.NoError(t, os.Rename(filepath.Join(dir, ".cursor-plugin"), target))
				return target
			},
		},
		{
			name:     "nested parent directory symlink",
			link:     "skills/core-s",
			wantLink: "skills/core-s",
			setup: func(t *testing.T, dir string) string {
				target := filepath.Join(dir, "elsewhere")
				require.NoError(t, os.Rename(filepath.Join(dir, "skills", "core-s"), target))
				return target
			},
		},
	}
}

// snapshot reads every regular file under root into a map.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	info, err := os.Stat(root)
	require.NoError(t, err)
	if !info.IsDir() {
		body, readErr := os.ReadFile(root)
		require.NoError(t, readErr)
		files[root] = string(body)
		return files
	}
	require.NoError(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		files[path] = string(body)
		return nil
	}))
	return files
}

func prepareLink(t *testing.T, tt linkCase) (dir, target string) {
	t.Helper()
	dir, _ = pruneProject(t)
	target = tt.setup(t, dir)
	link := filepath.Join(dir, filepath.FromSlash(tt.link))
	require.NoError(t, os.RemoveAll(link))
	testutil.SymlinkOrSkip(t, target, link)
	return dir, target
}

func TestGeneratePlugin_RefusesSymlinkedOutput(t *testing.T) {
	for _, tt := range linkCases() {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir, target := prepareLink(t, tt)
			before := snapshot(t, target)

			// Act
			err := loadDomainsProject(t, dir).GeneratePlugin("")

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantLink)
			assert.Contains(t, err.Error(), "symlink")
			assert.Equal(t, before, snapshot(t, target), "the link target never changes")
			info, lerr := os.Lstat(filepath.Join(dir, filepath.FromSlash(tt.link)))
			require.NoError(t, lerr)
			assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link stays")
		})
	}
}

func TestDryRunPlugin_RefusesSymlinkedOutput(t *testing.T) {
	for _, tt := range linkCases() {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir, target := prepareLink(t, tt)
			before := snapshot(t, target)

			// Act
			_, err := loadDomainsProject(t, dir).DryRunPlugin("")

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantLink)
			assert.Equal(t, before, snapshot(t, target))
		})
	}
}

func TestVerifyPlugin_ReportsSymlinkedOutputPath(t *testing.T) {
	for _, tt := range linkCases() {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir, _ := prepareLink(t, tt)

			// Act
			err := loadDomainsProject(t, dir).VerifyPlugin("")

			// Assert
			require.ErrorIs(t, err, ErrPluginDrift)
			assert.Contains(t, PluginDriftOf(err), PluginDrift{Path: tt.wantLink, Kind: PluginDriftSymlink})
		})
	}
}
