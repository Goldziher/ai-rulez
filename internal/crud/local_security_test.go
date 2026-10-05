package crud_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
)

func TestLocalOperator_IgnoresLocalTreeBeforeWriting(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		add    func(op *crud.OperatorImpl, req *crud.AddFileRequest) (*crud.FileResult, error)
	}{
		{"rule", "", func(op *crud.OperatorImpl, r *crud.AddFileRequest) (*crud.FileResult, error) {
			return op.AddRule(context.Background(), r)
		}},
		{"skill in a domain", "team", func(op *crud.OperatorImpl, r *crud.AddFileRequest) (*crud.FileResult, error) {
			return op.AddSkill(context.Background(), r)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			shared, cfgDir := localOperator(t)
			base := filepath.Dir(cfgDir)

			// Act
			_, err := tt.add(shared.Local(), &crud.AddFileRequest{Name: "mine", Domain: tt.domain, Content: "token SECRET"})

			// Assert
			require.NoError(t, err)
			ignore, err := os.ReadFile(filepath.Join(base, ".gitignore"))
			require.NoError(t, err)
			assert.Contains(t, string(ignore), ".ai-rulez/local/")
		})
	}
}

func TestLocalOperator_FailsClosedWhenGitignoreIsUnwritable(t *testing.T) {
	// Arrange: a directory where .gitignore should be makes the ignore step fail
	shared, cfgDir := localOperator(t)
	base := filepath.Dir(cfgDir)
	require.NoError(t, os.Mkdir(filepath.Join(base, ".gitignore"), 0o755))

	// Act
	_, err := shared.Local().AddRule(context.Background(), &crud.AddFileRequest{Name: "mine", Content: "token SECRET"})

	// Assert
	require.Error(t, err)
	assert.NoDirExists(t, filepath.Join(cfgDir, "local"))
}

func TestLocalOperator_WritesOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	// Arrange
	shared, cfgDir := localOperator(t)

	// Act
	res, err := shared.Local().AddRule(context.Background(), &crud.AddFileRequest{Name: "mine", Domain: "", Content: "x"})

	// Assert
	require.NoError(t, err)
	info, err := os.Stat(res.FullPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	dir, err := os.Stat(filepath.Join(cfgDir, "local", "rules"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dir.Mode().Perm())
	top, err := os.Stat(filepath.Join(cfgDir, "local"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), top.Mode().Perm())
}

func TestFileManager_AtomicWriteIgnoresPredictableTempFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	// Arrange: a symlink where the old predictable temp file lived
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.txt")
	require.NoError(t, os.WriteFile(victim, []byte("keep"), 0o600))
	target := filepath.Join(dir, "rule.md")
	require.NoError(t, os.Symlink(victim, target+".tmp"))
	fm := crud.NewFileManager(dir)

	// Act
	err := fm.WriteFile(target, "new content")

	// Assert
	require.NoError(t, err)
	data, err := os.ReadFile(victim)
	require.NoError(t, err)
	assert.Equal(t, "keep", string(data), "the symlinked temp path must not be written through")
	written, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new content", string(written))
}
