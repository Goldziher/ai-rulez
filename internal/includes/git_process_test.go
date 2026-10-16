package includes

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitFetchProcess(t *testing.T) {
	repo := os.Getenv("AI_RULEZ_TEST_FETCH_REPO")
	if repo == "" {
		t.Skip("subprocess helper")
	}
	source, err := NewGitSource("shared", "file://"+repo, "", os.Getenv("AI_RULEZ_TEST_FETCH_SHA"), repo, nil, "")
	require.NoError(t, err)
	source.cacheDir = os.Getenv("AI_RULEZ_TEST_FETCH_CACHE")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for {
		if _, err := os.Stat(os.Getenv("AI_RULEZ_TEST_FETCH_START")); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	tree, err := source.Fetch(ctx)
	require.NoError(t, err)
	require.Len(t, tree.Rules, 1)
	require.Equal(t, "Shared rule.\n", strings.ReplaceAll(tree.Rules[0].Content, "\r\n", "\n"), "a Windows checkout may convert line endings")
}

func TestGitFetchSerializesSeparateProcesses(t *testing.T) {
	repo := initLocalRepo(t)
	commitFile(t, repo, ".gitattributes", "* text eol=lf\n")
	commitFile(t, repo, ".ai-rulez/rules/shared.md", "---\nname: shared\n---\nShared rule.\n")
	root := t.TempDir()
	start := filepath.Join(root, "start")
	exe, err := os.Executable()
	require.NoError(t, err)
	const workers = 8
	cmds := make([]*exec.Cmd, workers)
	logs := make([]*os.File, workers)
	for i := range cmds {
		logs[i], err = os.CreateTemp(root, "result-")
		require.NoError(t, err)
		t.Cleanup(func() { _ = logs[i].Close() })
		cmds[i] = exec.Command(exe, "-test.run=^TestGitFetchProcess$", "-test.count=1")
		cmds[i].Env = append(os.Environ(), "AI_RULEZ_TEST_FETCH_REPO="+repo,
			"AI_RULEZ_TEST_FETCH_SHA="+revParseHead(t, repo), "AI_RULEZ_TEST_FETCH_CACHE="+filepath.Join(root, "checkout"),
			"AI_RULEZ_TEST_FETCH_START="+start)
		cmds[i].Stdout, cmds[i].Stderr = logs[i], logs[i]
		require.NoError(t, cmds[i].Start())
	}
	require.NoError(t, os.WriteFile(start, nil, 0o600))
	completed := 0
	for i, cmd := range cmds {
		err := cmd.Wait()
		body, readErr := os.ReadFile(logs[i].Name())
		require.NoError(t, readErr)
		assert.NoError(t, err, "worker %d: %s", i, body)
		completed++
	}
	require.Equal(t, workers, completed)
}

func TestGitCacheLockHonorsCancellation(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "checkout")
	release, err := lockFetchCache(context.Background(), cache)
	require.NoError(t, err)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = lockFetchCache(ctx, cache)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	releaseAfterWait, err := lockFetchCache(context.Background(), filepath.Join(t.TempDir(), "different-checkout"))
	require.NoError(t, err)
	releaseAfterWait()
}
