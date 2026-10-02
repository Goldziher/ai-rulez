package includes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveLockFile_RedactsCredentials(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, SaveLockFile(dir, &Lock{Includes: map[string]LockEntry{
		"a": CreateLockEntry("git", "https://user:pw@example.com/o/r.git", "x"),
		"b": CreateLockEntry("git", "https://x-access-token:ghp_secret@github.com/o/p.git", "y"),
	}}))

	data, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "includes.lock"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "pw@")
	assert.NotContains(t, string(data), "ghp_secret")
	assert.Contains(t, string(data), "example.com/o/r.git")
}
