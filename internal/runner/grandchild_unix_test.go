//go:build !windows

package runner

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// grandchildPID waits for the script to record the pid of the process it left running.
func grandchildPID(t *testing.T, pidfile string) int {
	t.Helper()
	var pid int
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if data, err := os.ReadFile(pidfile); err == nil && strings.HasSuffix(string(data), "\n") {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(data))) //nolint:errcheck // checked below
			break
		}
	}
	if pid == 0 {
		t.Fatal("the script never recorded its grandchild")
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) }) //nolint:errcheck // leave nothing behind
	return pid
}

func TestRunKillsTheGrandchildWhenTheChildEnds(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		timeout    time.Duration
		wantStatus Status
	}{
		{"timeout", "sleep 30 &\necho $! > \"$PIDFILE\"\nwait\n", 2 * time.Second, StatusTimeout},
		{"exit zero while the grandchild holds stdout", "sleep 30 &\necho $! > \"$PIDFILE\"\nprintf hi\nexit 0\n", 0, StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: the grandchild records its pid at once and would outlive the child by 30s.
			old := killGrace
			killGrace = 300 * time.Millisecond
			t.Cleanup(func() { killGrace = old })
			pidfile := filepath.Join(t.TempDir(), "pid")
			env := ScrubEnv(os.Environ(), nil, []string{"PIDFILE=" + pidfile})
			start := time.Now()

			// Act
			res := Run(context.Background(), Spec{Argv: []string{script(t, tt.body)}, Env: env, Timeout: tt.timeout})

			// Assert
			if res.Status != tt.wantStatus {
				t.Fatalf("status = %s (%v), want %s", res.Status, res.Err, tt.wantStatus)
			}
			if time.Since(start) > 15*time.Second {
				t.Errorf("Run took %s; it waited for the grandchild", time.Since(start))
			}
			pid := grandchildPID(t, pidfile)
			deadline := time.Now().Add(10 * time.Second)
			for !gone(pid) && time.Now().Before(deadline) {
				time.Sleep(20 * time.Millisecond)
			}
			if !gone(pid) {
				t.Error("grandchild survived Run")
			}
		})
	}
}
