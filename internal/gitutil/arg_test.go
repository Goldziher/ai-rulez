package gitutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
