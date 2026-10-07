package airulez_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/pkg/airulez"
)

// includeRepo is a git repository holding one shared rule, to be named as a
// file:// include.
func includeRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".ai-rulez", "rules", "shared.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("# Shared\n\nBe kind.\n"), 0o644))
	git := func(args ...string) {
		all := append([]string{"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
		cmd := gitutil.CommandNoContext(dir, all...)
		cmd.Env = append(gitutil.Env(nil), "HOME="+t.TempDir(), "GIT_CONFIG_NOSYSTEM=1")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	git("init", "-q", "-b", "main", ".")
	git("add", "-A")
	git("commit", "-q", "-m", "shared")
	return dir
}

// TestRemoteLoadHonoursAnEnforcedLock is RV-ENGINE-1: an ai-rulez.lock turns
// [lock] enforce on, so a remote include the lock does not cover must fail the
// facade's load exactly as it fails `ai-rulez generate`, instead of being
// fetched unpinned and generated from.
func TestRemoteLoadHonoursAnEnforcedLock(t *testing.T) {
	tests := []struct {
		name      string
		lockTable string
		lockFile  bool
		violation bool
	}{
		{name: "a lock that does not cover the include is a violation", lockFile: true, violation: true},
		{name: "no lock file fetches the include unpinned", lockFile: false},
		{name: "enforce = false opts out", lockFile: true, lockTable: "\n[lock]\nenforce = false\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
			repo := includeRepo(t)
			dir := t.TempDir()
			cfg := "version = \"4.0\"\nname = \"p\"\npresets = [\"claude\"]\n\n[[includes]]\nname = \"b\"\nsource = \"file://" +
				filepath.ToSlash(repo) + "\"\n" + tt.lockTable
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte(cfg), 0o644))
			if tt.lockFile {
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "ai-rulez.lock"), []byte("version = 1\n"), 0o644))
			}
			ws, err := airulez.DirWorkspace(dir)
			require.NoError(t, err)

			// Act
			p, err := airulez.Load(context.Background(), airulez.Options{Workspace: ws, Remote: true, Runner: &execRunner{},
				Env: airulez.MapEnv{Home: home, Vars: map[string]string{"HOME": home, "XDG_CACHE_HOME": filepath.Join(home, ".cache")}}})

			// Assert
			if tt.violation {
				require.Error(t, err)
				require.ErrorIs(t, err, config.ErrLockViolation)
				assert.Contains(t, err.Error(), "not covered by ai-rulez.lock")
				return
			}
			require.NoError(t, err)
			res, err := p.Generate(context.Background(), airulez.GenerateOptions{Mode: airulez.Write})
			require.NoError(t, err)
			assert.Positive(t, res.Written)
		})
	}
}
