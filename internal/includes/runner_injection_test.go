package includes

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

func TestRemoteHEADSHAUsesTheRunnerOfTheContext(t *testing.T) {
	// Arrange
	const sha = "0123456789abcdef0123456789abcdef01234567"
	fake := &runner.Fake{Handle: func(runner.Spec) runner.Result {
		return runner.Result{Status: runner.StatusOK, Stdout: []byte(sha + "\trefs/heads/main\n")}
	}}
	ctx := runner.WithContext(context.Background(), fake)

	// Act
	got, err := remoteHEADSHA(ctx, "https://example.com/org/repo.git", "main", "")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, sha, got)
	calls := fake.Calls()
	require.Len(t, calls, 1)
	assert.Equal(t, "git", calls[0].Argv[0])
	assert.Contains(t, strings.Join(calls[0].Argv, " "), "ls-remote -- https://example.com/org/repo.git refs/heads/main")
}

func TestDeniedRunnerStopsAnIncludeFetch(t *testing.T) {
	// Arrange
	ctx := runner.WithContext(context.Background(), runner.Deny{})

	// Act
	_, err := remoteHEADSHA(ctx, "https://example.com/org/repo.git", "main", "")
	cloneErr := sparseClone(ctx, "https://example.com/org/repo.git", "main", "", t.TempDir()+"/dest", "")

	// Assert
	require.Error(t, err)
	require.Error(t, cloneErr)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestRequireGitIsNotCachedForAnInjectedRunner(t *testing.T) {
	// Arrange
	fake := &runner.Fake{Handle: func(runner.Spec) runner.Result {
		return runner.Result{Status: runner.StatusOK, Stdout: []byte("git version 2.43.0\n")}
	}}
	ctx := runner.WithContext(context.Background(), fake)

	// Act
	first, second := requireGit(ctx), requireGit(ctx)

	// Assert
	require.NoError(t, first)
	require.NoError(t, second)
	assert.Len(t, fake.Calls(), 2, "an injected runner answers every probe itself")
}
