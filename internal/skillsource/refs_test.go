package skillsource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// refs.json is read back as a cache path component in offline mode, so a value
// that is not a commit id must count as never resolved.
func TestReadRef_RejectsAValueThatIsNotACommit(t *testing.T) {
	dir := t.TempDir()
	good := strings.Repeat("ab", 20)
	writeRef(dir, "v1", good)
	assert.Equal(t, good, readRef(dir, "v1"))

	for _, bad := range []string{"../../../etc", "ABCDEF", strings.Repeat("a", 39), ""} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "refs.json"), []byte(`{"`+refLabel("v1")+`":"`+bad+`"}`), 0o600))
		assert.Empty(t, readRef(dir, "v1"), bad)
	}

	writeRef(dir, "v2", good)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp", "no temp file is left behind")
	}
}
