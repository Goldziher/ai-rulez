package usage

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

// assertPerm checks the permission bits of info. Windows has none to check: Go
// reports 0666 (0777 for a directory) for anything writable, and ACLs keep the
// per-user files private.
func assertPerm(t *testing.T, want os.FileMode, info os.FileInfo) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	assert.Equal(t, want, info.Mode().Perm())
}
