package workspace_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

// projectIn makes <repo>/<rel>/.ai-rulez/config.toml and returns the project directory.
func projectIn(t *testing.T, repo, rel string) string {
	t.Helper()
	dir := filepath.Join(repo, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte("version = \"5.0\"\n"), 0o644))
	return dir
}

// An enclosing repository bounds a project's symlinks only when it holds the
// project: a project under a dotfiles repository in $HOME is not part of it, and
// widening the boundary to $HOME would let a symlink read any file of the user.
func TestAroundRootsAtTheEnclosingRepositoryOnlyWhenItTracksTheProject(t *testing.T) {
	needGit(t)
	tests := []struct {
		name     string
		tracked  bool
		wantRoot func(repo, project string) string
	}{
		{"a project the repository tracks", true, func(repo, _ string) string { return repo }},
		{"a project the repository does not track", false, func(_, project string) string { return project }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			repo := t.TempDir()
			runGit(t, repo, "init", "-q", ".")
			project := projectIn(t, repo, "svc/api")
			if tt.tracked {
				runGit(t, repo, "add", "svc/api/.ai-rulez/config.toml")
			}

			// Act
			ws, err := workspace.Around(project)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantRoot(repo, project), ws.Root())
		})
	}
}

func TestAroundKeepsTheRepositoryRootForTheProjectAtItsTop(t *testing.T) {
	needGit(t)
	// Arrange
	repo := t.TempDir()
	runGit(t, repo, "init", "-q", ".")
	projectIn(t, repo, ".")

	// Act
	ws, err := workspace.Around(repo)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, repo, ws.Root())
}

// git resolves the symlinks of GIT_CEILING_DIRECTORIES before comparing; so does
// the workspace, or a ceiling spelled through a link would be ignored and a
// repository above it found.
func TestAroundBelowResolvesSymlinksInCeilings(t *testing.T) {
	needGit(t)
	// Arrange: <repo>/.git tracks the project; the ceiling is <repo> reached through a link
	repo := t.TempDir()
	runGit(t, repo, "init", "-q", ".")
	project := projectIn(t, repo, "svc/api")
	runGit(t, repo, "add", "svc/api/.ai-rulez/config.toml")
	link := filepath.Join(t.TempDir(), "alias")
	testutil.SymlinkOrSkip(t, repo, link)
	realProject, err := filepath.EvalSymlinks(project)
	require.NoError(t, err)

	// Act
	ws, err := workspace.AroundBelow(t.Context(), gitutil.Git{}, realProject, link)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, realProject, ws.Root(), "the ceiling, whatever its spelling, is not searched")
}
