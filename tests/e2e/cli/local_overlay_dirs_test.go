package cli

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

// #499: directory outputs of a preset only the overlay adds (.gemini/agents/) are
// machine-local; the managed .gitignore block is the same with and without it.
func TestLocalOverlayDirectoryOutputsStayOutOfTheSharedBlock(t *testing.T) {
	for _, gitignore := range []bool{true, false} {
		t.Run("gitignore="+strconv.FormatBool(gitignore), func(t *testing.T) {
			dir := testutil.CreateTempDir(t)
			require.NoError(t, exec.Command("git", "-C", dir, "init", "-q").Run())
			env := map[string]string{"AI_RULEZ_ACK_COMMANDS": "1"}
			writeRel(t, dir, ".ai-rulez/config.toml", "version = \"5.0\"\nname = \"c\"\ngitignore = "+
				strconv.FormatBool(gitignore)+"\npresets = [\"claude\"]\n")
			writeRel(t, dir, ".ai-rulez/rules/style.md", "---\npriority: high\n---\n\nUse tabs.\n")
			require.Equal(t, 0, testutil.RunCLIWithEnv(t, dir, env, "generate", "--yes", "-q").ExitCode)
			shared, _ := readIfExists(t, filepath.Join(dir, ".gitignore"))

			writeRel(t, dir, ".ai-rulez/config.local.toml", "version = \"5.0\"\npresets = [\"claude\", \"gemini\"]\n")
			res := testutil.RunCLIWithEnv(t, dir, env, "generate", "--yes")

			require.Equal(t, 0, res.ExitCode, res.Stdout+res.Stderr)
			withOverlay, _ := readIfExists(t, filepath.Join(dir, ".gitignore"))
			assert.NotContains(t, withOverlay, ".gemini")
			if gitignore {
				assert.Equal(t, shared, withOverlay)
			}
			exclude := testutil.ReadFile(t, filepath.Join(dir, ".git", "info", "exclude"))
			assert.Contains(t, exclude, "\n/.gemini/agents/\n")
			status, err := exec.Command("git", "-C", dir, "status", "--porcelain", "--untracked-files=all").Output()
			require.NoError(t, err)
			assert.NotContains(t, string(status), ".gemini")
		})
	}
}
