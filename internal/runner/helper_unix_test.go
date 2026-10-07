//go:build !windows

package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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

// helperMore handles the modes of later tests.
func helperMore(string, []string) int { return 2 }
