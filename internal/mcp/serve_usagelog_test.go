package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A load_skill appends to --usage-log (and creates usage.salt beside it). When
// those files sit under a watched root the fingerprint must not see them, or
// every load would rebuild the whole catalog.
func TestServeSetup_WatcherFingerprintIgnoresTheConfiguredUsageLog(t *testing.T) {
	root := project(t, baseConfig, map[string]string{
		"skills/a/SKILL.md": skillFile("a", "Served skill", "delivery: served\n"),
	})
	logPath := filepath.Join(root, ".ai-rulez", "usage.log")
	require.NoError(t, os.WriteFile(logPath, []byte("one\n"), 0o600))

	setup := &ServeSetup{WorkDir: root, UsageLog: logPath, CacheDir: filepath.Join(t.TempDir(), "cache")}
	srv, err := setup.NewServer(context.Background())
	require.NoError(t, err)
	opts := srv.serve.opts
	require.NotNil(t, opts.Fingerprint)
	require.NotEmpty(t, opts.Baseline)

	cur, err := opts.Fingerprint()
	require.NoError(t, err)
	assert.Equal(t, opts.Baseline, cur, "the watcher must agree with the baseline before any load")

	require.NoError(t, os.WriteFile(logPath, []byte("one\ntwo\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(logPath), "usage.salt"), []byte("salt"), 0o600))
	cur, err = opts.Fingerprint()
	require.NoError(t, err)
	assert.Equal(t, opts.Baseline, cur, "a load_skill record must not look like a skill change")

	writeFile(t, root, ".ai-rulez/skills/a/SKILL.md", skillFile("a", "Changed description", "delivery: served\n"))
	cur, err = opts.Fingerprint()
	require.NoError(t, err)
	assert.NotEqual(t, opts.Baseline, cur, "a real edit must still be seen")
}

func TestServeSetup_RelativeUsageLogIsIgnoredToo(t *testing.T) {
	root := project(t, baseConfig, map[string]string{
		"skills/a/SKILL.md": skillFile("a", "Served skill", "delivery: served\n"),
	})
	t.Chdir(root)
	logRel := filepath.Join(".ai-rulez", "usage.log")
	require.NoError(t, os.WriteFile(logRel, []byte("one\n"), 0o600))
	setup := &ServeSetup{WorkDir: root, UsageLog: logRel, CacheDir: filepath.Join(t.TempDir(), "cache")}
	srv, err := setup.NewServer(context.Background())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(logRel, []byte("one\ntwo\n"), 0o600))
	cur, err := srv.serve.opts.Fingerprint()
	require.NoError(t, err)
	assert.Equal(t, srv.serve.opts.Baseline, cur)
}
