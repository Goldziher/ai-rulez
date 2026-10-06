package safefs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func skipIfNoSymlinks(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs privileges on windows")
	}
}

func TestOpenAppend_RefusesSymlinks(t *testing.T) {
	skipIfNoSymlinks(t)
	tests := []struct {
		name  string
		build func(t *testing.T, root string) (path, victim string)
	}{
		{"file is a symlink", func(t *testing.T, root string) (string, string) {
			victim := filepath.Join(root, "victim.txt")
			require.NoError(t, os.WriteFile(victim, []byte("keep\n"), 0o600))
			local := filepath.Join(root, ".ai-rulez", "local")
			require.NoError(t, os.MkdirAll(local, 0o750))
			testutil.SymlinkOrSkip(t, victim, filepath.Join(local, "usage.jsonl"))
			return filepath.Join(local, "usage.jsonl"), victim
		}},
		{"dangling symlink", func(t *testing.T, root string) (string, string) {
			local := filepath.Join(root, ".ai-rulez", "local")
			require.NoError(t, os.MkdirAll(local, 0o750))
			victim := filepath.Join(root, "created")
			testutil.SymlinkOrSkip(t, victim, filepath.Join(local, "usage.jsonl"))
			return filepath.Join(local, "usage.jsonl"), victim
		}},
		{"immediate directory is a symlink", func(t *testing.T, root string) (string, string) {
			target := filepath.Join(root, "elsewhere")
			require.NoError(t, os.MkdirAll(target, 0o750))
			require.NoError(t, os.MkdirAll(filepath.Join(root, ".ai-rulez"), 0o750))
			testutil.SymlinkOrSkip(t, target, filepath.Join(root, ".ai-rulez", "local"))
			return filepath.Join(root, ".ai-rulez", "local", "usage.jsonl"), filepath.Join(target, "usage.jsonl")
		}},
		{"ancestor .ai-rulez is a symlink", func(t *testing.T, root string) (string, string) {
			target := filepath.Join(root, "elsewhere")
			require.NoError(t, os.MkdirAll(filepath.Join(target, "local"), 0o750))
			testutil.SymlinkOrSkip(t, target, filepath.Join(root, ".ai-rulez"))
			return filepath.Join(root, ".ai-rulez", "local", "usage.jsonl"), filepath.Join(target, "local", "usage.jsonl")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path, victim := tt.build(t, t.TempDir())
			before, _ := os.ReadFile(victim) //nolint:errcheck // may not exist

			// Act
			file, err := OpenAppend(path)

			// Assert
			if file != nil {
				_ = file.Close() //nolint:errcheck // test
			}
			require.ErrorContains(t, err, "refusing")
			after, _ := os.ReadFile(victim) //nolint:errcheck // may not exist
			assert.Equal(t, string(before), string(after))
		})
	}
}

func TestOpenAppend_CreatesAndAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ai-rulez", "local", "a", "u.jsonl")
	for _, line := range []string{"one\n", "two\n"} {
		require.NoError(t, AppendLine(path, []byte(line)))
	}
	data, err := os.ReadFile(path) //nolint:gosec // test path
	require.NoError(t, err)
	assert.Equal(t, "one\ntwo\n", string(data))
}

func TestWriteFileAtomic_ReplacesASymlinkNotItsTarget(t *testing.T) {
	skipIfNoSymlinks(t)
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	require.NoError(t, os.WriteFile(victim, []byte("keep"), 0o600))
	marker := filepath.Join(dir, "marker")
	testutil.SymlinkOrSkip(t, victim, marker)

	require.NoError(t, WriteFileAtomic(marker, nil))

	data, _ := os.ReadFile(victim) //nolint:errcheck // test
	assert.Equal(t, "keep", string(data))
	info, err := os.Lstat(marker)
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular())
}

func TestReadRegular_RefusesSymlink(t *testing.T) {
	skipIfNoSymlinks(t)
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	require.NoError(t, os.WriteFile(victim, []byte("secret"), 0o644))
	link := filepath.Join(dir, "link")
	testutil.SymlinkOrSkip(t, victim, link)

	_, err := ReadRegular(link)
	require.ErrorContains(t, err, "refusing")
	info, _ := os.Stat(victim) //nolint:errcheck // test
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm(), "the target's mode must not be touched")

	data, err := ReadRegular(victim)
	require.NoError(t, err)
	assert.Equal(t, "secret", string(data))
}

func TestAppendLine_RelativePathUnderConfigDir(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, AppendLine(filepath.Join(".ai-rulez", "local", "usage.jsonl"), []byte("one\n")))
	data, err := os.ReadFile(filepath.Join(".ai-rulez", "local", "usage.jsonl"))
	require.NoError(t, err)
	assert.Equal(t, "one\n", string(data))
}
