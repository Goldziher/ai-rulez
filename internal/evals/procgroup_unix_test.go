//go:build !windows

package evals

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readPid returns the pid the script wrote, polling until the file holds a
// complete line or the bound passes.
func readPid(pidFile string) (int, bool) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(pidFile); err == nil && strings.HasSuffix(string(data), "\n") {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				return pid, true
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return 0, false
}

// cancelWhenReady cancels the run once the child has written its pid, so the
// kill under test always lands on a live process tree, however loaded the machine is.
func cancelWhenReady(pidFile string, cancel context.CancelFunc) {
	go func() {
		readPid(pidFile)
		cancel()
	}()
}

// processGone waits, bounded, for the pid in pidFile to stop existing (ESRCH).
func processGone(t *testing.T, pidFile string) bool {
	t.Helper()
	pid, ok := readPid(pidFile)
	require.True(t, ok, "the child never wrote its pid")
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestCommandRunner_TimeoutKillsBackgroundChildren(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	runner := &CommandRunner{Command: "sleep 60 & echo $! > " + pidFile + "; wait", Timeout: time.Minute}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelWhenReady(pidFile, cancel)
	start := time.Now()
	_, err := runner.Run(ctx, &Request{Version: ProtocolVersion, Skill: SkillRef{ID: "s"}})
	require.Error(t, err)
	assert.Less(t, time.Since(start), 45*time.Second, "the run must not hang on a child that holds stdout")
	assert.True(t, processGone(t, pidFile), "the background child must be killed with the runner")
}

func TestCommandAssertion_TimeoutKillsBackgroundChildren(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	a := &Assertion{Type: AssertCommandExit, Command: "sleep 60 & echo $! > " + pidFile + "; wait"}
	start := time.Now()
	msg := runCommandAssertion(a, t.TempDir(), GradeOptions{AllowExec: true, CommandTimeout: 3 * time.Second})
	assert.NotEmpty(t, msg)
	assert.Less(t, time.Since(start), 45*time.Second)
	assert.True(t, processGone(t, pidFile))
}
