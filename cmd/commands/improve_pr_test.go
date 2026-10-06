package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/improve"
)

func readTestFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return string(b)
}

func TestImprovePR_OpensAPullRequestWithAFakeGH(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	// Arrange: an accepted run in a committed repository with a local bare origin and a fake gh on PATH.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	root := setupImproveRun(t)
	improveChildSelf(t)
	var out, errOut bytes.Buffer
	improveRunCmd.SetOut(&out)
	improveRunCmd.SetErr(&errOut)
	_, err := runImprove(improveRunCmd, "deploy")
	require.NoError(t, err)
	var report improve.Report
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	run := func(dir string, args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "commit.gpgsign=false", "-c", "user.email=t@example.test", "-c", "user.name=t"}, args...)...) //nolint:gosec // test
		b, gerr := cmd.CombinedOutput()
		require.NoError(t, gerr, string(b))
		return strings.TrimSpace(string(b))
	}
	run(root, "init", "-q", "-b", "main")
	run(root, "add", ".ai-rulez/config.toml", ".ai-rulez/skills")
	run(root, "commit", "-q", "-m", "init")
	remote := filepath.Join(t.TempDir(), "origin.git")
	run(root, "init", "-q", "--bare", remote)
	run(root, "remote", "add", "origin", remote)
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$(dirname \"$0\")/gh.log\"\necho https://github.com/example/repo/pull/1\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755)) //nolint:gosec // a test script
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	improveFlags.format, improveFlags.yes = formatJSON, true
	improvePRFlags.draft = true
	t.Cleanup(func() { improvePRFlags.draft = false })
	var prOut bytes.Buffer
	improvePRCmd.SetOut(&prOut)
	improvePRCmd.SetErr(&errOut)

	// Act
	require.NoError(t, improvePRCmd.RunE(improvePRCmd, []string{report.RunID}), errOut.String())

	// Assert: stdout is the result document; gh got fixed arguments; the checkout is untouched.
	var res improve.PRResult
	require.NoError(t, json.Unmarshal(prOut.Bytes(), &res))
	assert.True(t, res.Pushed)
	assert.Equal(t, "https://github.com/example/repo/pull/1", res.URL)
	logged := readTestFile(t, filepath.Join(bin, "gh.log"))
	assert.Contains(t, logged, "pr\ncreate\n--base\nmain\n--head\n"+res.Branch+"\n")
	assert.True(t, strings.HasSuffix(strings.TrimSpace(logged), "--draft"))
	assert.Equal(t, res.Commit, run(remote, "rev-parse", "refs/heads/"+res.Branch))
	assert.Equal(t, improveSkillBody, readTestFile(t, filepath.Join(root, ".ai-rulez/skills/deploy/SKILL.md")))
	assert.Empty(t, run(root, "status", "--porcelain", "--", ".ai-rulez/skills"))
}

func TestImprovePR_RefusesAnUnknownRun(t *testing.T) {
	// Arrange
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	resetImproveFlags(t)
	improveProject(t)
	improvePRCmd.SetOut(&bytes.Buffer{})
	improvePRCmd.SetErr(&bytes.Buffer{})

	// Act
	err := improvePRCmd.RunE(improvePRCmd, []string{"imp-00000000"})

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no saved run")
}
