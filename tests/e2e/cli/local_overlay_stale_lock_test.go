package cli

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve/tagtest"
	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

// #507: an overlay that replaces a remote include with a local path must not
// need a fresh lock when the drift check is waived, and its outputs stay
// machine-local.
func TestLocalOverlayWithStaleLock(t *testing.T) {
	setup := func(t *testing.T, gitignore bool) (dir string, env map[string]string) {
		repo := tagtest.New(t)
		repo.Write(".ai-rulez/rules/shared.md", includeRule("Shared", "shared", "SHARED_ONE"))
		first := repo.Commit("one")
		repo.Write(".ai-rulez/rules/shared.md", includeRule("Shared", "shared", "SHARED_TWO"))
		second := repo.Commit("two")

		dir = testutil.CreateTempDir(t)
		require.NoError(t, exec.Command("git", "-C", dir, "init", "-q").Run())
		env = map[string]string{"AI_RULEZ_ALLOW_FILE_URLS": "1", "AI_RULEZ_ACK_COMMANDS": "1"}
		write := func(ref string) {
			writeRel(t, dir, ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"c\"\ngitignore = "+
				strconv.FormatBool(gitignore)+"\npresets = [\"claude\"]\n\n[[includes]]\nname = \"core\"\nsource = '"+
				repo.URL+"'\nref = \""+ref+"\"\n")
		}
		write(first)
		require.Equal(t, 0, testutil.RunCLIWithEnv(t, dir, env, "lock", "-q").ExitCode)
		require.Equal(t, 0, testutil.RunCLIWithEnv(t, dir, env, "generate", "--yes", "-q").ExitCode)
		write(second) // a pending bump: the lock is now stale

		writeRel(t, dir, "shared/.ai-rulez/rules/shared.md", includeRule("Shared", "shared", "SHARED_LOCAL"))
		writeRel(t, dir, ".ai-rulez/config.local.toml",
			"version = \"5.0\"\npresets = [\"claude\", \"gemini\"]\n\n[[includes]]\nname = \"core\"\nsource = \"./shared\"\n")
		return dir, env
	}

	for _, gitignore := range []bool{true, false} {
		t.Run("allow-local-drift keeps overlay outputs machine-local, gitignore="+strconv.FormatBool(gitignore),
			func(t *testing.T) {
				dir, env := setup(t, gitignore)

				res := testutil.RunCLIWithEnv(t, dir, env, "generate", "--yes", "--allow-local-drift")

				require.Equal(t, 0, res.ExitCode, res.Stdout+res.Stderr)
				assert.Contains(t, testutil.ReadFile(t, filepath.Join(dir, "AGENTS.md")), "SHARED_LOCAL")
				assert.Contains(t, res.Stdout+res.Stderr, "lock is stale")
				exclude := testutil.ReadFile(t, filepath.Join(dir, ".git", "info", "exclude"))
				assert.Contains(t, exclude, "/.gemini/settings.json")
				assert.Contains(t, exclude, "\n/.gemini/agents/\n")
				status, err := exec.Command("git", "-C", dir, "status", "--porcelain", "--untracked-files=all").Output()
				require.NoError(t, err)
				assert.NotContains(t, string(status), ".gemini")
				if gitignore {
					assert.NotContains(t, testutil.ReadFile(t, filepath.Join(dir, ".gitignore")), ".gemini")
				}
			})
	}

	t.Run("without the flag the error points at it", func(t *testing.T) {
		dir, env := setup(t, false)

		res := testutil.RunCLIWithEnv(t, dir, env, "generate", "--yes")

		require.Equal(t, 1, res.ExitCode, res.Stdout+res.Stderr)
		out := res.Stdout + res.Stderr
		assert.Contains(t, out, "shared baseline")
		assert.Contains(t, out, "--allow-local-drift")
	})
}
