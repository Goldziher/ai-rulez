//go:build !windows

package evals

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func processGone(t *testing.T, pidFile string) bool {
	t.Helper()
	data, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	for range 50 {
		if err := syscall.Kill(pid, 0); err != nil {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func TestCommandRunner_TimeoutKillsBackgroundChildren(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	runner := &CommandRunner{Command: "sleep 60 & echo $! > " + pidFile + "; wait", Timeout: 500 * time.Millisecond}
	start := time.Now()
	_, err := runner.Run(context.Background(), &Request{Version: ProtocolVersion, Skill: SkillRef{ID: "s"}})
	require.Error(t, err)
	assert.Less(t, time.Since(start), 10*time.Second, "the run must not hang on a child that holds stdout")
	assert.True(t, processGone(t, pidFile), "the background child must be killed with the runner")
}

func TestCommandAssertion_TimeoutKillsBackgroundChildren(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	a := &Assertion{Type: AssertCommandExit, Command: "sleep 60 & echo $! > " + pidFile + "; wait"}
	start := time.Now()
	msg := runCommandAssertion(a, t.TempDir(), GradeOptions{AllowExec: true, CommandTimeout: 500 * time.Millisecond})
	assert.NotEmpty(t, msg)
	assert.Less(t, time.Since(start), 10*time.Second)
	assert.True(t, processGone(t, pidFile))
}

func TestKillTreeOnCancel_GroupAlreadyGoneIsNotAFailure(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "true")
	killTreeOnCancel(cmd)
	require.NoError(t, cmd.Run())

	err := cmd.Cancel()

	assert.False(t, errors.Is(err, syscall.ESRCH), "ESRCH must not surface: %v", err)
	if err != nil {
		assert.ErrorIs(t, err, os.ErrProcessDone)
	}
}
