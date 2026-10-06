package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLockSubject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	resetLockViewFlags(t)
	root := lockProject(t, "")
	require.Equal(t, 0, writeLockAt("", "", nil))
	t.Cleanup(func() { lockSubject, lockSubjectOutput, lockFormat = false, "", "" })
	lock, err := lockfile.Load(filepath.Join(root, ".ai-rulez"))
	require.NoError(t, err)
	want := contentlock.SubjectOf(lock).Digest()

	t.Run("text prints the digest and what it commits to", func(t *testing.T) {
		var code int
		stdout := captureStdout(t, func() { code = lockSubjectAt("") })

		assert.Equal(t, 0, code)
		assert.Contains(t, stdout, want)
		assert.Contains(t, stdout, "tree "+lock.Tree)
	})

	t.Run("json is deterministic and written to --output", func(t *testing.T) {
		lockFormat = formatJSON
		defer func() { lockFormat = "" }()
		out := filepath.Join(t.TempDir(), "subject.json")
		lockSubjectOutput = out
		defer func() { lockSubjectOutput = "" }()

		var code int
		_, _ = capture(t, func() { code = lockSubjectAt("") })
		first, err := os.ReadFile(out)
		require.NoError(t, err)
		_, _ = capture(t, func() { code = lockSubjectAt("") })
		second, err := os.ReadFile(out)
		require.NoError(t, err)

		assert.Equal(t, 0, code)
		assert.Equal(t, first, second)
		var doc contentlock.SubjectStatement
		require.NoError(t, json.Unmarshal(first, &doc))
		assert.Equal(t, want, doc.Subject)
		assert.Equal(t, 1, doc.SchemaVersion)
		assert.Equal(t, 1, doc.HashVersion)
		assert.Empty(t, doc.ApprovalsDigest)
	})

	t.Run("an edited pin is refused", func(t *testing.T) {
		path := lockfile.Path(filepath.Join(root, ".ai-rulez"))
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		defer func() { require.NoError(t, os.WriteFile(path, data, 0o644)) }()
		edited := *lock // Save keeps the stale tree, so the entries no longer match it
		edited.Item = append([]lockfile.Item(nil), lock.Item...)
		edited.Item[0].Digest = "sha256:" + strings.Repeat("00", 32)
		require.NoError(t, lockfile.Save(filepath.Join(root, ".ai-rulez"), &edited))

		var code int
		_, stderr := capture(t, func() { code = lockSubjectAt("") })

		assert.Equal(t, exitDrift, code)
		assert.Contains(t, stderr, "does not match its entries")
	})

	t.Run("a missing lock is an error", func(t *testing.T) {
		require.NoError(t, os.Remove(lockfile.Path(filepath.Join(root, ".ai-rulez"))))

		var code int
		_, stderr := capture(t, func() { code = lockSubjectAt("") })

		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, "no content pins")
	})
}
