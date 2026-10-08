package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/stretchr/testify/require"
)

// WriteTree writes files (slash-separated paths relative to root), creating the
// directories on the way.
func WriteTree(tb testing.TB, root string, files map[string]string) {
	tb.Helper()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(tb, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(tb, os.WriteFile(full, []byte(content), 0o644)) //nolint:gosec // test fixture
	}
}

// Git runs git in dir with a fixed identity and signing off, fails the test on a
// non-zero exit and returns the trimmed combined output.
func Git(tb testing.TB, dir string, args ...string) string {
	tb.Helper()
	// runner.CommandNoContext, not gitutil.Command: gitutil's own tests import this package.
	cmd := runner.CommandNoContext("git", append([]string{"-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(tb, err, string(out))
	return strings.TrimSpace(string(out))
}
