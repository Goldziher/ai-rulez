package airulez_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/pkg/airulez"
)

// okfIncludeRepo is a git repository holding an OKF bundle whose only concept
// carries body, to be included with format = "okf".
func okfIncludeRepo(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kb", "notes.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("---\ntype: Decision\ndescription: Notes\n---\n"+body), 0o644))
	git := func(args ...string) {
		all := append([]string{"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
		cmd := gitutil.CommandNoContext(dir, all...)
		cmd.Env = append(gitutil.Env(nil), "HOME="+t.TempDir(), "GIT_CONFIG_NOSYSTEM=1")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	git("init", "-q", "-b", "main", ".")
	git("add", "-A")
	git("commit", "-q", "-m", "kb")
	return dir
}

// TestAnOKFIncludeIsSecurityScannedThroughTheFacade is #290 item 1: an OKF
// include an embedding service loads must run the same security scan the CLI
// runs, so injected content is refused instead of silently imported.
func TestAnOKFIncludeIsSecurityScannedThroughTheFacade(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "a secret is refused", body: "key AKIAIOSFODNN7EXAMPLE\n", wantErr: true},
		{name: "benign text loads", body: "Use tabs for indentation.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: a project whose OKF include is a git repository.
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
			repo := okfIncludeRepo(t, tt.body)
			dir := t.TempDir()
			cfg := "version = \"5.0\"\nname = \"p\"\npresets = [\"claude\"]\n\n[[includes]]\nname = \"kb\"\nsource = \"file://" +
				filepath.ToSlash(repo) + "\"\npath = \"kb\"\nformat = \"okf\"\n"
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte(cfg), 0o644))
			ws, err := airulez.DirWorkspace(dir)
			require.NoError(t, err)

			// Act
			_, err = airulez.Load(context.Background(), airulez.Options{Workspace: ws, Remote: true, Runner: &execRunner{},
				Env: airulez.MapEnv{Home: home, Vars: map[string]string{"HOME": home, "XDG_CACHE_HOME": filepath.Join(home, ".cache")}}})

			// Assert
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "security scan")
		})
	}
}
