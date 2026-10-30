package generator

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// TestUpdateGitignore_AnchorsRootFiles is the #515 regression: a managed entry
// for a root file must not hide a tracked file of the same name deeper in the
// repository, such as a plugin bundle's .mcp.json or a nested AGENTS.md.
func TestUpdateGitignore_AnchorsRootFiles(t *testing.T) {
	// Arrange: the bundle the repository commits lives under plugin/.
	dir := gitRepo(t, "")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "plugin", "sub"), 0o755))
	for _, rel := range []string{"plugin/.mcp.json", "plugin/sub/AGENTS.md"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte("{}\n"), 0o644))
	}
	gitCmd(t, dir, "add", "-A")
	gen := NewGenerator(&config.Config{BaseDir: dir, Gitignore: gitignoreOn()})

	// Act
	require.NoError(t, gen.updateGitignore([]config.OutputFile{
		{Path: filepath.Join(dir, ".mcp.json")},
		{Path: filepath.Join(dir, "AGENTS.md")},
	}))

	// Assert
	inside := gitignore.PatternsInsideFence(readGitignore(t, dir))
	assert.Contains(t, inside, "/.mcp.json")
	assert.Contains(t, inside, "/AGENTS.md")
	for _, rel := range []string{"plugin/.mcp.json", "plugin/sub/AGENTS.md"} {
		cmd := gitutil.CommandNoContext(dir, "check-ignore", "--no-index", rel)
		out, err := cmd.CombinedOutput()
		code := 0
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		}
		assert.Equal(t, 1, code, "%s must stay visible to git, got %q (%v)", rel, out, err)
	}
}

// TestUpdateGitignore_IgnoresMachineLocalExcludes is the #518 regression: the
// committed block is the same whatever the developer's global excludes file and
// .git/info/exclude say.
func TestUpdateGitignore_IgnoresMachineLocalExcludes(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
	}{
		{
			name: "info/exclude",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "info", "exclude"), []byte("CLAUDE.md\n"), 0o644))
			},
		},
		{
			name: "global excludes file",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				path := filepath.Join(t.TempDir(), "global-ignore")
				require.NoError(t, os.WriteFile(path, []byte("AGENTS.md\n"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "gitconfig"),
					[]byte("[core]\n\texcludesFile = "+filepath.ToSlash(path)+"\n"), 0o644))
				t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, "gitconfig"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := gitRepo(t, "")
			tt.setup(t, dir)
			gen := NewGenerator(&config.Config{BaseDir: dir, Gitignore: gitignoreOn()})

			// Act
			require.NoError(t, gen.updateGitignore(gitignoreOutputs(dir)))

			// Assert: both names stay in the block, so a teammate without the rule is covered.
			inside := strings.Join(gitignore.PatternsInsideFence(readGitignore(t, dir)), "\n")
			assert.Contains(t, inside, "/CLAUDE.md")
			assert.Contains(t, inside, "/AGENTS.md")
		})
	}
}
