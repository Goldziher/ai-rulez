package mcp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// worktreeFixture is a repository with a committed AI-Rulez configuration and
// a managed ignore block, plus one linked worktree git created.
type worktreeFixture struct {
	root, src, linked string
}

func worktreeGitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@example.com", "-c", "user.name=t"}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func worktreeWrite(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func newWorktreeFixture(t *testing.T) *worktreeFixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	src := filepath.Join(root, "main")
	worktreeWrite(t, filepath.Join(src, ".ai-rulez/config.toml"),
		"version = \"5.0\"\nname = \"t\"\ngitignore = false\nsession_start = { generate = false }\npresets = [\"claude\"]\n")
	worktreeWrite(t, filepath.Join(src, ".gitignore"),
		"# BEGIN ai-rulez (DO NOT EDIT - managed by ai-rulez)\n"+
			".ai-rulez/config.local.toml\n.ai-rulez/local/\nCLAUDE.local.md\n# END ai-rulez\n")
	worktreeGitRun(t, src, "init", "-q")
	worktreeGitRun(t, src, "commit", "-q", "--allow-empty", "-m", "init")
	worktreeGitRun(t, src, "add", ".ai-rulez/config.toml", ".gitignore")
	worktreeGitRun(t, src, "commit", "-q", "-m", "config")
	linked := filepath.Join(root, "linked")
	worktreeGitRun(t, src, "worktree", "add", "-q", linked, "-b", "feature")
	return &worktreeFixture{root: root, src: src, linked: linked}
}

// resolved resolves symlinks, matching the source path the service reports
// (the project root comes back through discovery, which resolves). The
// destination stays the caller's path made absolute, as filepath.Abs gives it.
func resolved(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	require.NoError(t, err)
	out, err := filepath.EvalSymlinks(abs)
	require.NoError(t, err)
	return out
}

// absPath makes a path absolute without resolving symlinks.
func absPath(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	require.NoError(t, err)
	return abs
}

func worktreeSession(t *testing.T, f *worktreeFixture) *sdkmcp.ClientSession {
	t.Helper()
	srv := NewServer("test", WithRoot(f.src))
	return connect(t, srv)
}

func TestWorktreeSetupTool(t *testing.T) {
	t.Run("valid setup regenerates the destination", func(t *testing.T) {
		f := newWorktreeFixture(t)
		session := worktreeSession(t, f)

		res := call(t, session, "worktree_setup", map[string]any{"working_directory": f.src, "destination": f.linked, "dry_run": false})
		require.False(t, res.IsError, textOfResult(t, res))
		doc := structured(t, res)

		assert.Equal(t, true, doc["success"])
		assert.Equal(t, resolved(t, f.src), doc["source"])
		assert.Equal(t, absPath(t, f.linked), doc["destination"])
		assert.Equal(t, true, doc["generated"])
		assert.FileExists(t, filepath.Join(f.linked, "CLAUDE.md"), "the destination outputs were regenerated")
	})

	t.Run("dry run writes nothing", func(t *testing.T) {
		f := newWorktreeFixture(t)
		worktreeWrite(t, filepath.Join(f.src, "CLAUDE.local.md"), "# local\n")
		session := worktreeSession(t, f)

		res := call(t, session, "worktree_setup", map[string]any{"working_directory": f.src, "destination": f.linked, "dry_run": true, "copy_local": true})
		require.False(t, res.IsError, textOfResult(t, res))
		doc := structured(t, res)

		assert.Equal(t, true, doc["dry_run"])
		copies := doc["copies"].([]any)
		require.Len(t, copies, 1)
		first := copies[0].(map[string]any)
		assert.Equal(t, "copy", first["action"])
		assert.NoFileExists(t, filepath.Join(f.linked, "CLAUDE.local.md"), "a dry run writes nothing")
	})

	t.Run("local copying needs explicit consent", func(t *testing.T) {
		f := newWorktreeFixture(t)
		worktreeWrite(t, filepath.Join(f.src, "CLAUDE.local.md"), "# local\n")
		session := worktreeSession(t, f)

		res := call(t, session, "worktree_setup", map[string]any{"working_directory": f.src, "destination": f.linked})
		require.False(t, res.IsError, textOfResult(t, res))
		doc := structured(t, res)

		copies, _ := doc["copies"].([]any)
		assert.Empty(t, copies, "without copy_local nothing is carried")
		assert.NoFileExists(t, filepath.Join(f.linked, "CLAUDE.local.md"))
	})

	t.Run("consented copying carries the allowlisted inputs", func(t *testing.T) {
		f := newWorktreeFixture(t)
		worktreeWrite(t, filepath.Join(f.src, "CLAUDE.local.md"), "# local\n")
		session := worktreeSession(t, f)

		res := call(t, session, "worktree_setup", map[string]any{"working_directory": f.src, "destination": f.linked, "copy_local": true, "dry_run": false})
		require.False(t, res.IsError, textOfResult(t, res))
		doc := structured(t, res)

		copies := doc["copies"].([]any)
		assert.Len(t, copies, 1)
		data, err := os.ReadFile(filepath.Join(f.linked, "CLAUDE.local.md"))
		require.NoError(t, err)
		assert.Equal(t, "# local\n", string(data))
	})

	t.Run("conflicting destination files refuse the run", func(t *testing.T) {
		f := newWorktreeFixture(t)
		worktreeWrite(t, filepath.Join(f.src, "CLAUDE.local.md"), "# source\n")
		worktreeWrite(t, filepath.Join(f.linked, "CLAUDE.local.md"), "# the agent's own edit\n")
		session := worktreeSession(t, f)

		res := call(t, session, "worktree_setup", map[string]any{"working_directory": f.src, "destination": f.linked, "copy_local": true})
		require.True(t, res.IsError, "a conflict refuses the run")
		assert.Contains(t, textOfResult(t, res), "differ from what the source would carry")

		data, err := os.ReadFile(filepath.Join(f.linked, "CLAUDE.local.md"))
		require.NoError(t, err)
		assert.Equal(t, "# the agent's own edit\n", string(data), "the destination edit is preserved")
	})

	t.Run("an unrelated repository is refused", func(t *testing.T) {
		f := newWorktreeFixture(t)
		other := filepath.Join(f.root, "other")
		worktreeWrite(t, filepath.Join(other, "README"), "x\n")
		worktreeGitRun(t, other, "init", "-q")
		session := worktreeSession(t, f)

		res := call(t, session, "worktree_setup", map[string]any{"working_directory": f.src, "destination": other})
		require.True(t, res.IsError)
		assert.Contains(t, textOfResult(t, res), "not a registered linked worktree")
	})

	t.Run("a read-only server does not expose the tool", func(t *testing.T) {
		f := newWorktreeFixture(t)
		srv := NewServer("test", WithRoot(f.src), WithReadOnly())
		session := connect(t, srv)

		_, err := session.CallTool(context.Background(), &sdkmcp.CallToolParams{
			Name: "worktree_setup", Arguments: map[string]any{"working_directory": f.src, "destination": f.linked},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown tool", "the tool is not registered on a read-only server")
	})
}

func TestWorktreeSetupDryRunSemantics(t *testing.T) {
	t.Run("an omitted dry_run plans instead of writing", func(t *testing.T) {
		f := newWorktreeFixture(t)
		worktreeWrite(t, filepath.Join(f.src, "CLAUDE.local.md"), "# local\n")
		session := worktreeSession(t, f)

		res := call(t, session, "worktree_setup", map[string]any{"working_directory": f.src, "destination": f.linked, "copy_local": true})
		require.False(t, res.IsError, textOfResult(t, res))
		doc := structured(t, res)

		assert.Equal(t, true, doc["dry_run"], "an omitted dry_run plans")
		assert.NoFileExists(t, filepath.Join(f.linked, "CLAUDE.local.md"))
	})

	t.Run("an explicit dry_run false writes", func(t *testing.T) {
		f := newWorktreeFixture(t)
		worktreeWrite(t, filepath.Join(f.src, "CLAUDE.local.md"), "# local\n")
		session := worktreeSession(t, f)

		res := call(t, session, "worktree_setup", map[string]any{
			"working_directory": f.src, "destination": f.linked, "copy_local": true, "dry_run": false,
		})
		require.False(t, res.IsError, textOfResult(t, res))
		doc := structured(t, res)

		_, planned := doc["dry_run"]
		assert.False(t, planned, "a real run does not advertise dry_run")
		assert.FileExists(t, filepath.Join(f.linked, "CLAUDE.local.md"))
	})

	t.Run("a relative destination resolves beside the source checkout", func(t *testing.T) {
		f := newWorktreeFixture(t)
		session := worktreeSession(t, f)
		// cwd of the test process is the package directory, not the fixture;
		// a relative destination must not land beside it.
		rel, err := filepath.Rel(cwd(t), f.linked)
		require.NoError(t, err)

		res := call(t, session, "worktree_setup", map[string]any{"working_directory": f.src, "destination": rel, "dry_run": false})
		require.False(t, res.IsError, textOfResult(t, res))
		doc := structured(t, res)
		assert.Equal(t, absPath(t, f.linked), doc["destination"])
		assert.FileExists(t, filepath.Join(f.linked, "CLAUDE.md"))
	})
}

// cwd is the test process's working directory, which is never the fixture.
func cwd(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	return dir
}

func TestWorktreeDestinationGeneratorStaysOffline(t *testing.T) {
	f := newWorktreeFixture(t)
	// The destination's configuration carries a remote include that nothing
	// ever cached, on an unroutable address. Online fetching would fail the
	// run (an unresolved include is refused unless the run is explicitly
	// offline); a provisioning run resolves includes from the local cache
	// only, so it succeeds without reaching the network.
	worktreeWrite(t, filepath.Join(f.linked, ".ai-rulez/config.toml"),
		"version = \"5.0\"\nname = \"t\"\ngitignore = false\nsession_start = { generate = false }\npresets = [\"claude\"]\n"+
			"[[includes]]\nname = \"remote\"\nsource = \"https://127.0.0.1:1/none\"\n")
	worktreeGitRun(t, f.linked, "add", ".ai-rulez/config.toml")
	worktreeGitRun(t, f.linked, "commit", "-q", "-m", "remote include")

	session := worktreeSession(t, f)
	res := call(t, session, "worktree_setup", map[string]any{"working_directory": f.src, "destination": f.linked, "dry_run": false})
	require.False(t, res.IsError, textOfResult(t, res))
	text := textOfResult(t, res)
	assert.Contains(t, text, "generated", "the run succeeded without fetching")
	assert.NotContains(t, text, "Always run the tests", "no generated file content is reported")
}
