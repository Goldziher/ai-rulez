package testutil

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type CLIResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

func RunCLI(t *testing.T, workingDir string, args ...string) *CLIResult {
	return RunCLIWithEnv(t, workingDir, nil, args...)
}

func RunCLIWithEnv(t *testing.T, workingDir string, env map[string]string, args ...string) *CLIResult {
	t.Helper()

	binaryPath := SetupTestBinary(t)

	//nolint:gosec // G204: Test utility needs to run subprocess with variables
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = workingDir

	if env != nil {
		cmdEnv := os.Environ()
		filtered := make([]string, 0, len(cmdEnv))
		for _, e := range cmdEnv {
			shouldKeep := true
			for k := range env {
				if strings.HasPrefix(e, k+"=") {
					shouldKeep = false
					break
				}
			}
			if shouldKeep {
				filtered = append(filtered, e)
			}
		}
		for k, v := range env {
			if v != "" {
				filtered = append(filtered, fmt.Sprintf("%s=%s", k, v))
			}
		}
		cmd.Env = filtered
	}

	done := make(chan *CLIResult, 1)

	go func() {
		stdout, stderr, exitCode, err := runCommand(cmd)
		done <- &CLIResult{
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: exitCode,
			Err:      err,
		}
	}()

	select {
	case result := <-done:
		return result
	case <-time.After(TestTimeout):
		//nolint:errcheck,gosec
		cmd.Process.Kill()
		t.Fatalf("Command timed out after %v: %v", TestTimeout, args)
		return nil
	}
}

func RunCLIExpectSuccess(t *testing.T, workingDir string, args ...string) *CLIResult {
	t.Helper()

	result := RunCLI(t, workingDir, args...)
	require.NoError(t, result.Err, "CLI command should succeed: %v\nStderr: %s", args, result.Stderr)
	require.Equal(t, 0, result.ExitCode, "CLI command should exit with code 0: %v\nStderr: %s", args, result.Stderr)

	return result
}

func RunCLIExpectError(t *testing.T, workingDir string, args ...string) *CLIResult {
	t.Helper()

	result := RunCLI(t, workingDir, args...)
	require.NotEqual(t, 0, result.ExitCode, "CLI command should fail: %v\nStdout: %s", args, result.Stdout)

	return result
}

func (r *CLIResult) AssertOutputContains(t *testing.T, expected string) {
	t.Helper()

	combined := r.Stdout + r.Stderr
	require.Contains(t, combined, expected, "Output should contain '%s'\nActual output: %s", expected, combined)
}

func (r *CLIResult) AssertStdoutContains(t *testing.T, expected string) {
	t.Helper()

	require.Contains(t, r.Stdout, expected, "Stdout should contain '%s'\nActual stdout: %s", expected, r.Stdout)
}

func (r *CLIResult) AssertStderrContains(t *testing.T, expected string) {
	t.Helper()

	require.Contains(t, r.Stderr, expected, "Stderr should contain '%s'\nActual stderr: %s", expected, r.Stderr)
}

func runCommand(cmd *exec.Cmd) (stdout, stderr string, exitCode int, err error) {
	var outBuffer, errBuffer strings.Builder
	cmd.Stdout = &outBuffer
	cmd.Stderr = &errBuffer

	err = cmd.Run()
	stdout = outBuffer.String()
	stderr = errBuffer.String()

	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
		}
	} else {
		exitCode = 0
	}

	return stdout, stderr, exitCode, err
}
