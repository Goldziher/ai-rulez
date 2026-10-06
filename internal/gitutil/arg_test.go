package gitutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

func TestCheckRemoteURL_RejectsPlainHTTPWithAMigrationNote(t *testing.T) {
	for _, u := range []string{"http://example.com/o/r.git", "HTTP://example.com/o/r", "git+http://example.com/o/r.git", " http://example.com/o/r"} {
		err := CheckRemoteURL("include url", u)
		require.Error(t, err, u)
		assert.Contains(t, err.Error(), "plain http://")
		assert.Contains(t, err.Error(), "use https://")
	}
	for _, u := range []string{"https://example.com/o/r.git", "git+https://example.com/o/r.git", "git@github.com:o/r.git", "ssh://git@example.com/o/r.git", "file:///tmp/repo", "/tmp/repo", "../repo"} {
		assert.NoError(t, CheckRemoteURL("include url", u), u)
	}
	assert.Error(t, CheckRemoteURL("include url", "--upload-pack=x"), "still refuses what CheckArg refuses")
}

func TestCheckRemoteURL_RejectsPlainGitProtocol(t *testing.T) {
	for _, u := range []string{"git://github.com/o/r", "GIT://example.com/o/r.git", "git+git://example.com/o/r"} {
		err := CheckRemoteURL("include url", u)
		require.Error(t, err, u)
		assert.Contains(t, err.Error(), "plain git://")
	}
}

func TestShowFile_RefusesARefThatLooksLikeAnOption(t *testing.T) {
	tests := []struct {
		name string
		ref  string
	}{
		{"option", "--output=/tmp/x"},
		{"short option", "-p"},
		{"empty", ""},
		{"blank", "  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &runner.Fake{Handle: func(runner.Spec) runner.Result {
				return runner.Result{Status: runner.StatusOK, Stdout: []byte("content")}
			}}

			got, ok := New(fake).ShowFile("/p", tt.ref, "ai-rulez.lock")

			assert.False(t, ok)
			assert.Nil(t, got)
			assert.Empty(t, fake.Calls(), "git must not run with an option-like revision")
		})
	}
}

func TestShowFile_PassesAnOrdinaryRefAsOneArgument(t *testing.T) {
	fake := &runner.Fake{Handle: func(runner.Spec) runner.Result {
		return runner.Result{Status: runner.StatusOK, Stdout: []byte("content")}
	}}

	got, ok := New(fake).ShowFile("/p", "v1.3.0", "dir/ai-rulez.lock")

	assert.True(t, ok)
	assert.Equal(t, "content", string(got))
	require.Len(t, fake.Calls(), 1)
	assert.Equal(t, []string{"git", "-C", "/p", "show", "v1.3.0:dir/ai-rulez.lock"}, fake.Calls()[0].Argv)
}
