package generator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const driftShared = `version = "5.0"
name = "shared-project"
presets = ["claude"]
gitignore = false
agents_md = false

[header]
hashes = "full"
`

type driftProject struct {
	base string
	dir  string
}

// newDriftProject writes a shared config (and one rule) into a fresh project.
func newDriftProject(t *testing.T, shared string) *driftProject {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(shared), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rules", "style.md"),
		[]byte("---\npriority: high\n---\n\nUse tabs.\n"), 0o600))
	return &driftProject{base: base, dir: dir}
}

func (p *driftProject) overlay(t *testing.T, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(p.dir, "config.local.toml"), []byte(body), 0o600))
}

func (p *driftProject) load(t *testing.T, opts ...config.LoadOption) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), p.base, opts...)
	require.NoError(t, err)
	return cfg
}

func (p *driftProject) read(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(p.base, filepath.FromSlash(rel)))
	require.NoError(t, err)
	return string(data)
}

func (p *driftProject) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(p.base, filepath.FromSlash(rel)))
	return err == nil
}

func (p *driftProject) git(t *testing.T, args ...string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	cmd := exec.Command("git", append([]string{"-C", p.base, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...) //nolint:gosec // test
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func headerHash(t *testing.T, content, name string) string {
	t.Helper()
	for _, line := range strings.Split(content, "\n") {
		if i := strings.Index(line, name+":"); i >= 0 {
			return strings.TrimSpace(strings.TrimSuffix(line[i+len(name)+1:], "-->"))
		}
	}
	return ""
}

func TestGenerate_DriftOnCommittedOutputErrors(t *testing.T) {
	// Arrange: the overlay renames the project, which changes the shared CLAUDE.md,
	// and nothing ignores that file.
	p := newDriftProject(t, driftShared)
	p.overlay(t, "name = \"secret-local-name\"\n")
	gen := NewGenerator(p.load(t))

	// Act
	err := gen.Generate("")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local overrides would change")
	rendered := err.Error()
	for _, line := range errorLines(err) {
		rendered += line
	}
	assert.Contains(t, rendered, "CLAUDE.md")
	assert.NotContains(t, rendered, "secret-local-name", "the error lists paths, never content")
	assert.False(t, p.exists("CLAUDE.md"), "nothing is written when the guard trips")
}

func TestGenerate_DriftIsFineWhenTheOutputIsIgnoredAndUntracked(t *testing.T) {
	// Arrange
	p := newDriftProject(t, strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1))
	p.overlay(t, "name = \"mine\"\n")

	// Act
	err := NewGenerator(p.load(t)).Generate("")

	// Assert
	require.NoError(t, err)
	assert.Contains(t, p.read(t, "CLAUDE.md"), "mine")
}

func TestGenerate_DriftOnTrackedOutputErrorsEvenWhenIgnored(t *testing.T) {
	// Arrange
	p := newDriftProject(t, strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1))
	p.git(t, "init", "-q")
	require.NoError(t, NewGenerator(p.load(t, config.WithoutLocal())).Generate(""))
	p.git(t, "add", "-f", "CLAUDE.md")
	p.overlay(t, "name = \"mine\"\n")

	// Act
	err := NewGenerator(p.load(t)).Generate("")

	// Assert
	require.Error(t, err)
	assert.NotContains(t, p.read(t, "CLAUDE.md"), "mine")
}

func TestGenerate_AllowLocalDrift(t *testing.T) {
	// Arrange
	p := newDriftProject(t, driftShared)
	require.NoError(t, NewGenerator(p.load(t, config.WithoutLocal())).Generate(""))
	sharedHash := headerHash(t, p.read(t, "CLAUDE.md"), "Source-Hash")
	p.overlay(t, "name = \"mine\"\n")
	gen := NewGenerator(p.load(t))
	gen.SetAllowLocalDrift(true)

	// Act
	err := gen.Generate("")

	// Assert
	require.NoError(t, err)
	claude := p.read(t, "CLAUDE.md")
	assert.Contains(t, claude, "mine")
	assert.Equal(t, sharedHash, headerHash(t, claude, "Source-Hash"), "shared outputs keep the baseline Source-Hash")
}

func TestGenerate_NoLocalMatchesTeammate(t *testing.T) {
	// Arrange: a teammate without any overlay, and this machine with one that
	// only adds a preset (so shared outputs do not drift).
	teammate := newDriftProject(t, driftShared)
	require.NoError(t, NewGenerator(teammate.load(t)).Generate(""))

	mine := newDriftProject(t, driftShared)
	mine.overlay(t, "presets = [\"codex\"]\n")

	// Act
	require.NoError(t, NewGenerator(mine.load(t)).Generate(""))
	mineClaude := mine.read(t, "CLAUDE.md")
	require.NoError(t, NewGenerator(mine.load(t, config.WithoutLocal())).Generate(""))

	// Assert: a shared output is byte-identical with and without the overlay, and
	// --no-local reproduces the teammate view.
	assert.Equal(t, teammate.read(t, "CLAUDE.md"), mineClaude)
	assert.Equal(t, teammate.read(t, "CLAUDE.md"), mine.read(t, "CLAUDE.md"))
	assert.Equal(t, teammate.read(t, ".ai-rulez/.generated-manifest.json"), mine.read(t, ".ai-rulez/.generated-manifest.json"))
}

func TestGenerate_LocalOnlyOutputsGetTheirOwnSourceHash(t *testing.T) {
	// Arrange
	p := newDriftProject(t, driftShared)
	p.overlay(t, "presets = [\"codex\"]\n")

	// Act
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))

	// Assert
	sharedHash := headerHash(t, p.read(t, "CLAUDE.md"), "Source-Hash")
	localHash := headerHash(t, p.read(t, "AGENTS.md"), "Source-Hash")
	require.NotEmpty(t, sharedHash)
	require.NotEmpty(t, localHash)
	assert.NotEqual(t, sharedHash, localHash)
}

func TestStale_TeammateDoesNotDeleteOwnLocalFiles(t *testing.T) {
	// Arrange: the overlay adds a preset whose outputs exist only on this machine.
	p := newDriftProject(t, driftShared)
	p.overlay(t, "presets = [\"codex\"]\n")
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))
	require.True(t, p.exists("AGENTS.md"))
	shared := p.read(t, ".ai-rulez/.generated-manifest.json")
	local := p.read(t, ".ai-rulez/.generated-manifest.local.json")
	assert.NotContains(t, shared, "AGENTS.md", "the committed manifest lists shared outputs only")
	assert.Contains(t, local, "AGENTS.md")

	// Act: generating the shared view (as a teammate would) on the same machine.
	require.NoError(t, NewGenerator(p.load(t, config.WithoutLocal())).Generate(""))

	// Assert: the local file and its manifest survive.
	assert.True(t, p.exists("AGENTS.md"))
	assert.Equal(t, local, p.read(t, ".ai-rulez/.generated-manifest.local.json"))

	// And once the overlay is really gone, the next run cleans the local output up.
	require.NoError(t, os.Remove(filepath.Join(p.dir, "config.local.toml")))
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))
	assert.False(t, p.exists("AGENTS.md"))
	assert.False(t, p.exists(".ai-rulez/.generated-manifest.local.json"))
}

func TestGenerate_SuppressedSharedOutputIsNeitherWrittenNorDeleted(t *testing.T) {
	// Arrange
	p := newDriftProject(t, driftShared)
	require.NoError(t, NewGenerator(p.load(t, config.WithoutLocal())).Generate(""))
	claudeBefore := p.read(t, "CLAUDE.md")
	p.overlay(t, "presets = [\"!claude\", \"codex\"]\n")
	gen := NewGenerator(p.load(t))

	// Act
	plan, dryErr := gen.DryRun("")
	err := gen.Generate("")

	// Assert
	require.NoError(t, dryErr)
	assert.Contains(t, plan, "suppressed: CLAUDE.md")
	assert.Contains(t, plan, "local-only: AGENTS.md")
	require.NoError(t, err)
	assert.Equal(t, claudeBefore, p.read(t, "CLAUDE.md"), "a file only the shared view owns is left alone")
	assert.Contains(t, p.read(t, ".ai-rulez/.generated-manifest.json"), "CLAUDE.md")
}

func TestDryRun_ReportsLocalOnlyAndDrift(t *testing.T) {
	// Arrange
	p := newDriftProject(t, driftShared)
	p.overlay(t, "name = \"mine\"\npresets = [\"codex\"]\n")

	// Act
	plan, err := NewGenerator(p.load(t)).DryRun("")

	// Assert
	require.NoError(t, err)
	assert.Contains(t, plan, "local-only: AGENTS.md")
	assert.Contains(t, plan, "drift: CLAUDE.md")
	assert.True(t, containsPrefix(plan, "blocked: CLAUDE.md"), "dry-run shows what the guard would refuse: %v", plan)
}

func TestGitignore_MachineSpecificPathsGoToInfoExclude(t *testing.T) {
	// Arrange
	p := newDriftProject(t, strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1))
	p.git(t, "init", "-q")
	p.overlay(t, "presets = [\"codex\"]\n")

	// Act
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))

	// Assert
	exclude := p.read(t, ".git/info/exclude")
	assert.Contains(t, exclude, "/AGENTS.md")
	ignore := p.read(t, ".gitignore")
	assert.NotContains(t, ignore, "AGENTS.md", "the shared .gitignore stays machine-independent")
	assert.Contains(t, ignore, ".ai-rulez/config.local.*")
	assert.Contains(t, ignore, ".ai-rulez/.generated-manifest.local.json")
}

func TestGitignore_ExcludeIsWrittenAtomicallyKeepingUserLines(t *testing.T) {
	// Arrange
	p := newDriftProject(t, strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1))
	p.git(t, "init", "-q")
	infoDir := filepath.Join(p.base, ".git", "info")
	require.NoError(t, os.MkdirAll(infoDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(infoDir, "exclude"), []byte("# mine\n*.swp\n"), 0o644))
	p.overlay(t, "presets = [\"codex\"]\n")

	// Act
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))

	// Assert
	exclude := p.read(t, ".git/info/exclude")
	assert.Contains(t, exclude, "*.swp")
	assert.Contains(t, exclude, "/AGENTS.md")
	entries, err := os.ReadDir(infoDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp", "no temp file is left beside the exclude file")
	}
}

func TestGitignore_ExcludeBlockFollowsTheLocalSet(t *testing.T) {
	// Arrange
	p := newDriftProject(t, strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1))
	p.git(t, "init", "-q")
	p.overlay(t, "presets = [\"codex\"]\n")
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))

	// Act: the local preset goes away.
	p.overlay(t, "name = \"x\"\n")
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))

	// Assert
	assert.NotContains(t, p.read(t, ".git/info/exclude"), "AGENTS.md")
}

func TestGitignore_WorktreeUsesTheSharedInfoExclude(t *testing.T) {
	// Arrange: a committed project, checked out a second time as a linked worktree.
	p := newDriftProject(t, strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1))
	p.git(t, "init", "-q")
	p.git(t, "add", ".ai-rulez")
	p.git(t, "commit", "-q", "-m", "init")
	linked := filepath.Join(t.TempDir(), "wt")
	p.git(t, "worktree", "add", "-q", linked, "-b", "feature")
	wt := &driftProject{base: linked, dir: filepath.Join(linked, ".ai-rulez")}
	wt.overlay(t, "presets = [\"codex\"]\n")

	// Act
	require.NoError(t, NewGenerator(wt.load(t)).Generate(""))

	// Assert: the linked checkout has a .git file, and the exclude lands in the main repo's .git.
	assert.Contains(t, p.read(t, ".git/info/exclude"), "/AGENTS.md")
}

func TestGitignore_NotARepositoryFallsBackToGitignore(t *testing.T) {
	// Arrange
	p := newDriftProject(t, strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1))
	p.overlay(t, "presets = [\"codex\"]\n")

	// Act
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))

	// Assert
	assert.Contains(t, p.read(t, ".gitignore"), "AGENTS.md")
}

func TestGenerate_LocalServerSecretDoesNotBreakTheBaseline(t *testing.T) {
	// Arrange: the shared server needs ${SHARED_ONLY}, which this machine does not
	// have because its overlay replaces that server's env. The baseline render
	// must tolerate the unresolved placeholder.
	shared := strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1) + `
[[mcp_servers]]
name = "svc"
command = "svc"
[mcp_servers.env]
API_TOKEN = "${SHARED_ONLY}"
`
	p := newDriftProject(t, shared)
	p.overlay(t, "[[mcp_servers]]\nname = \"svc\"\n[mcp_servers.env]\nAPI_TOKEN = \"local-value\"\n")
	t.Setenv("SHARED_ONLY", "")
	require.NoError(t, os.Unsetenv("SHARED_ONLY"))

	// Act
	err := NewGenerator(p.load(t)).Generate("")

	// Assert
	require.NoError(t, err)
	assert.Contains(t, p.read(t, ".mcp.json"), "local-value")
}

func errorLines(err error) []string {
	type contexter interface{ Context() map[string]any }
	var out []string
	if c, ok := err.(contexter); ok { //nolint:errorlint // oops errors are not wrapped further
		if lines, ok := c.Context()["errors"].([]string); ok {
			out = lines
		}
	}
	return out
}

func containsPrefix(lines []string, prefix string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			return true
		}
	}
	return false
}

func TestClean_RemovesTheInfoExcludeBlock(t *testing.T) {
	// Arrange
	p := newDriftProject(t, strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1))
	p.git(t, "init", "-q")
	p.overlay(t, "presets = [\"codex\"]\n")
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))
	require.Contains(t, p.read(t, ".git/info/exclude"), "/AGENTS.md")

	t.Run("dry run keeps the block", func(t *testing.T) {
		_, err := NewGenerator(p.load(t)).Clean("", CleanOptions{DryRun: true})
		require.NoError(t, err)
		assert.Contains(t, p.read(t, ".git/info/exclude"), "/AGENTS.md")
	})

	t.Run("clean removes it", func(t *testing.T) {
		_, err := NewGenerator(p.load(t)).Clean("", CleanOptions{})
		require.NoError(t, err)
		exclude := p.read(t, ".git/info/exclude")
		assert.NotContains(t, exclude, "AGENTS.md")
		assert.NotContains(t, exclude, "ai-rulez local")
	})
}

func TestDryRunBlocked_FollowsTheDriftGuard(t *testing.T) {
	tests := []struct {
		name    string
		allow   bool
		wantErr bool
	}{
		{"blocked drift is reported", false, true},
		{"allowed drift is not", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := newDriftProject(t, driftShared)
			p.overlay(t, "name = \"mine\"\npresets = [\"codex\"]\n")
			gen := NewGenerator(p.load(t))
			gen.SetAllowLocalDrift(tt.allow)

			// Act
			_, err := gen.DryRun("")
			blocked := gen.DryRunBlocked()

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantErr, blocked != nil)
		})
	}
}
