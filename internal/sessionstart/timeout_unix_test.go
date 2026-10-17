//go:build unix

package sessionstart

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// TestRun_TimeoutKillsTheWholeProcessTree runs a real child that starts a grandchild and hangs, as a
// stuck network fetch would: the hook returns at the timeout, exits 0, and no process is left behind.
func TestRun_TimeoutKillsTheWholeProcessTree(t *testing.T) {
	h := newHarness(t, nil)
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	script := filepath.Join(t.TempDir(), "ai-rulez")
	body := "#!/bin/sh\n/bin/sleep 60 &\necho $! > " + pidFile + "\n/bin/sleep 60\n"
	require.NoError(t, os.WriteFile(script, []byte(body), 0o755))
	h.deps.Exe = script
	h.deps.Runner = runner.Exec{}

	start := time.Now()
	code := h.run(`{"source":"startup"}`, Overrides{Timeout: 1500 * time.Millisecond})

	assert.Equal(t, 0, code)
	assert.Less(t, time.Since(start), 15*time.Second, "the run is bounded by the timeout")
	assert.Contains(t, h.errw.String(), "generate failed: timed out after")
	assert.Equal(t, 1, strings.Count(h.errw.String(), "\n"))
	assert.Contains(t, h.out.String(), "failed (timed out after", "the briefing says generate failed")
	data, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	assert.Eventually(t, func() bool { return syscall.Kill(pid, 0) != nil }, 5*time.Second, 50*time.Millisecond,
		"the grandchild is gone")
}
