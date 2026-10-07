//go:build !windows

package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// helperEnv makes the test binary act as a child process: its value names what
// the child does (see helper).
const helperEnv = "AI_RULEZ_RUNNER_HELPER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		os.Exit(helper(mode, os.Args[1:]))
	}
	os.Exit(m.Run())
}

// helperSpec runs this test binary as a helper in mode with args.
func helperSpec(t *testing.T, mode string, args ...string) Spec {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Spec{Argv: append([]string{exe}, args...), Env: ScrubEnv(os.Environ(), nil, []string{helperEnv + "=" + mode})}
}

// helper is the child side of the tests in this package.
func helper(mode string, args []string) int {
	switch mode {
	case "ttyprobe":
		// What a confined scanner does before a TIOCSTI injection: open the
		// controlling terminal of its session.
		f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
		if err != nil {
			fmt.Print("none")
			return 0
		}
		_ = f.Close() //nolint:errcheck // probe
		fmt.Print("opened")
	case "run-ttyprobe":
		// A process that owns a terminal (an interactive ai-rulez) runs a command.
		exe, _ := os.Executable() //nolint:errcheck // the parent already ran it
		res := Run(context.Background(), Spec{Argv: []string{exe}, Env: ScrubEnv(os.Environ(), nil, []string{helperEnv + "=ttyprobe"})})
		fmt.Print(string(res.Stdout))
	default:
		return helperMore(mode, args)
	}
	return 0
}

func TestRunGivesTheChildNoControllingTerminal(t *testing.T) {
	// Arrange: an intermediate process whose controlling terminal is a fresh
	// pseudo-terminal, like ai-rulez run from an interactive shell.
	ptmx, name, err := openPTY()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer ptmx.Close() //nolint:errcheck // test
	tty, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("open %s: %v", name, err)
	}
	defer tty.Close() //nolint:errcheck // test
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe) //nolint:gosec // the test binary
	cmd.Env = append(os.Environ(), helperEnv+"=run-ttyprobe")
	cmd.Stdin = tty
	var out strings.Builder
	cmd.Stdout = &out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	// Act
	if err := cmd.Run(); err != nil {
		t.Fatalf("intermediate: %v", err)
	}
	// Assert: the command it ran cannot reach the terminal (no TIOCSTI target).
	if got := out.String(); got != "none" {
		t.Fatalf("child opened /dev/tty: got %q, want %q", got, "none")
	}
}

// helperMore handles the process-tree modes:
//   - linger: sleep, the helper that must not outlive the run;
//   - escapee <pidfile> <hold>: start a linger in a new session (setsid), write
//     its pid, then stay alive for hold;
//   - daemon <pidfile> <hold>: start an escapee (holding 300ms) in a new
//     session, a double fork, then stay alive for hold.
func helperMore(mode string, args []string) int {
	switch mode {
	case "linger":
		time.Sleep(30 * time.Second)
		return 0
	case "escapee", "daemon":
		if len(args) < 2 {
			return 2
		}
		hold, err := time.ParseDuration(args[1])
		if err != nil {
			return 2
		}
		exe, _ := os.Executable() //nolint:errcheck // the parent already ran it
		var c *exec.Cmd
		if mode == "escapee" {
			c = exec.Command(exe) //nolint:gosec // the test binary
			c.Env = append(os.Environ(), helperEnv+"=linger")
		} else {
			c = exec.Command(exe, args[0], "300ms") //nolint:gosec // the test binary
			c.Env = append(os.Environ(), helperEnv+"=escapee")
		}
		c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := c.Start(); err != nil {
			return 3
		}
		if mode == "escapee" {
			if err := os.WriteFile(args[0], []byte(strconv.Itoa(c.Process.Pid)), 0o600); err != nil {
				return 3
			}
		}
		time.Sleep(hold)
		return 0
	}
	return 2
}

// gone reports whether pid has exited (a zombie waiting for its new parent to
// reap it counts as exited).
func gone(pid int) bool {
	return syscall.Kill(pid, 0) != nil || isZombie(pid)
}

func TestRunKillsHelpersThatLeaveTheGroup(t *testing.T) {
	tests := []struct {
		name       string
		mode, hold string
		timeout    time.Duration
		wantStatus Status
	}{
		{"setsid helper at the timeout", "escapee", "30s", 500 * time.Millisecond, StatusTimeout},
		{"setsid helper after a clean exit", "escapee", "300ms", 10 * time.Second, StatusOK},
		{"double-forked daemon after a clean exit", "daemon", "800ms", 10 * time.Second, StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			pidfile := filepath.Join(t.TempDir(), "pid")
			spec := helperSpec(t, tt.mode, pidfile, tt.hold)
			spec.Timeout = tt.timeout
			// Act
			res := Run(context.Background(), spec)
			// Assert
			if res.Status != tt.wantStatus {
				t.Fatalf("status = %s (%v), want %s", res.Status, res.Err, tt.wantStatus)
			}
			var pid int
			for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
				if data, err := os.ReadFile(pidfile); err == nil && len(data) > 0 {
					pid, _ = strconv.Atoi(string(data)) //nolint:errcheck // checked below
					break
				}
			}
			if pid <= 1 {
				t.Fatal("the helper never reported its pid")
			}
			t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) }) //nolint:errcheck // leave nothing behind
			for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
				if gone(pid) {
					return
				}
			}
			t.Fatalf("helper %d outlived the run", pid)
		})
	}
}
