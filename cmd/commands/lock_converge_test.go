package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
)

// TestLock_ConvergesInOneRunWithAGitInclude pins the include before the served
// entries are computed: a fresh project's first `lock` used to write the served
// skill of an include with an empty commit and a tree that the second run
// changed.
func TestLock_ConvergesInOneRunWithAGitInclude(t *testing.T) {
	// Arrange
	t.Setenv("HOME", t.TempDir())
	cliLockPolicy.Mode, cliLockPolicy.Refresh, cliLockPolicy.Offline = includes.LockAuto, nil, false
	includes.ResetObserved()
	t.Cleanup(func() { cliLockPolicy.Mode, cliLockPolicy.Refresh, cliLockPolicy.Offline = includes.LockAuto, nil, false })
	remote := crossRepo(t, ".ai-rulez/skills/inc/SKILL.md",
		"---\nname: inc\ndescription: Shared skill. Use when sharing.\n---\n\nINC\n")
	root := lockProject(t, "\n[skills]\ndelivery = \"served\"\n\n[[includes]]\nname = \"gitinc\"\nsource = \"file://"+
		filepath.ToSlash(remote)+"\"\nref = \"v1\"\ninclude = [\"skills\"]\n")
	lockPath := lockfile.Path(filepath.Join(root, ".ai-rulez"))

	// Act
	require.Equal(t, 0, writeLockAt("", "", nil), "first lock")
	first, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	checkAfterFirst := checkLockAt("")
	includes.ResetObserved()
	require.Equal(t, 0, writeLockAt("", "", nil), "second lock")
	second, err := os.ReadFile(lockPath)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, 0, checkAfterFirst, "lock --check right after the first lock")
	assert.Equal(t, string(first), string(second), "a second lock must not change anything")
	lock, err := lockfile.Load(filepath.Join(root, ".ai-rulez"))
	require.NoError(t, err)
	var served *lockfile.Entry
	for i := range lock.Served {
		if lock.Served[i].Name == "inc" {
			served = &lock.Served[i]
		}
	}
	require.NotNil(t, served, "the include's skill is pinned as served")
	assert.NotEmpty(t, served.Commit)
	assert.Equal(t, lock.Find(lockfile.KindInclude, "gitinc").Commit, served.Commit)
}

func TestUpdateText_SaysServedPinsStayStaleUntilLock(t *testing.T) {
	// Arrange
	rep := &updateReport{Updates: []updateItem{{Kind: lockfile.KindSource, Name: "vendor", To: &tagresolve.TagRef{Tag: "v2", Commit: "abcdef0123456789"}}}}

	// Act
	out, _ := capture(t, func() { writeUpdateText(rep) })

	// Assert
	assert.Contains(t, out, "served-skill pins")
	assert.Contains(t, out, "ai-rulez lock")
}
