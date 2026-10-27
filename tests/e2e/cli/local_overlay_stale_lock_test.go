package cli

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve/tagtest"
	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

// #507: an overlay that replaces a remote include with a local path must not
// need a fresh lock when the drift check is waived.
func TestLocalOverlayWithStaleLock(t *testing.T) {
	setup := func(t *testing.T) (dir string, env map[string]string) {
		repo := tagtest.New(t)
		repo.Write(".ai-rulez/rules/shared.md", includeRule("Shared", "shared", "SHARED_ONE"))
		first := repo.Commit("one")
		repo.Write(".ai-rulez/rules/shared.md", includeRule("Shared", "shared", "SHARED_TWO"))
		second := repo.Commit("two")

		dir = testutil.CreateTempDir(t)
		env = map[string]string{"AI_RULEZ_ALLOW_FILE_URLS": "1", "AI_RULEZ_ACK_COMMANDS": "1"}
		write := func(ref string) {
			writeRel(t, dir, ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"c\"\ngitignore = false\n"+
				"presets = [\"claude\"]\n\n[[includes]]\nname = \"core\"\nsource = '"+repo.URL+"'\nref = \""+ref+"\"\n")
		}
		write(first)
		require.Equal(t, 0, testutil.RunCLIWithEnv(t, dir, env, "lock", "-q").ExitCode)
		require.Equal(t, 0, testutil.RunCLIWithEnv(t, dir, env, "generate", "--yes", "-q").ExitCode)
		write(second) // a pending bump: the lock is now stale

		writeRel(t, dir, "shared/.ai-rulez/rules/shared.md", includeRule("Shared", "shared", "SHARED_LOCAL"))
		writeRel(t, dir, ".ai-rulez/config.local.toml",
			"version = \"5.0\"\n\n[[includes]]\nname = \"core\"\nsource = \"./shared\"\n")
		return dir, env
	}

	t.Run("allow-local-drift generates from the local path", func(t *testing.T) {
		dir, env := setup(t)

		res := testutil.RunCLIWithEnv(t, dir, env, "generate", "--yes", "--allow-local-drift")

		require.Equal(t, 0, res.ExitCode, res.Stdout+res.Stderr)
		assert.Contains(t, testutil.ReadFile(t, filepath.Join(dir, "AGENTS.md")), "SHARED_LOCAL")
		assert.Contains(t, res.Stdout+res.Stderr, "drift check did not run")
		assert.Contains(t, res.Stdout+res.Stderr, "core")
	})

	t.Run("without the flag the error points at it", func(t *testing.T) {
		dir, env := setup(t)

		res := testutil.RunCLIWithEnv(t, dir, env, "generate", "--yes")

		require.Equal(t, 1, res.ExitCode, res.Stdout+res.Stderr)
		out := res.Stdout + res.Stderr
		assert.Contains(t, out, "shared baseline")
		assert.Contains(t, out, "--allow-local-drift")
	})
}
