// Package runner runs an external command with the limits a tool needs when it
// executes something it did not write: a scrubbed environment, a timeout that
// kills the whole process group, a cap on captured output, and a distinct
// outcome for a binary that is not installed. It never goes through a shell.
//
// The package carries no policy about what the command prints. Callers (lint
// scanners today; verifiers and plan/apply later) decide what an outcome means.
package runner

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Limits applied when a Spec leaves them unset or asks for more than allowed.
const (
	// DefaultTimeout is how long a command may run when Spec.Timeout is zero.
	DefaultTimeout = 2 * time.Minute
	// MaxTimeout is the longest timeout a Spec may request.
	MaxTimeout = 15 * time.Minute
	// DefaultMaxOutput is the per-stream capture cap in bytes.
	DefaultMaxOutput int64 = 32 << 20
)

// killGrace bounds how long Run waits for pipes to drain after the child exited
// or was killed, so a grandchild that kept a pipe open cannot hang the caller.
// It is a variable so tests can shorten it.
var killGrace = 2 * time.Second

// Status is the coarse outcome of a run.
type Status string

// Outcomes of Run.
const (
	// StatusOK means the command ran and exited with status 0.
	StatusOK Status = "ok"
	// StatusExit means the command ran and exited non-zero.
	StatusExit Status = "exit"
	// StatusTimeout means the command was killed at its timeout.
	StatusTimeout Status = "timeout"
	// StatusUnavailable means the executable was not found or is not executable.
	StatusUnavailable Status = "unavailable"
	// StatusError means the command could not be started or waited on for another reason.
	StatusError Status = "error"
)

// Spec describes one command. The zero value of every field except Argv is usable.
type Spec struct {
	// Argv is the command and its arguments; Argv[0] is looked up on PATH unless it contains a separator.
	Argv []string
	// Dir is the working directory; empty means the current directory.
	Dir string
	// Env is the complete environment (KEY=VALUE) of the child. It is ignored when InheritEnv is set.
	// Build it with ScrubEnv; an empty Env gives the child no environment at all.
	Env []string
	// InheritEnv passes the parent's full environment to the child. Prefer an explicit Env.
	InheritEnv bool
	// Stdin is the data piped to the child; nil connects it to the null device.
	Stdin []byte
	// Timeout bounds the run; zero means DefaultTimeout and values above MaxTimeout are lowered to it.
	Timeout time.Duration
	// MaxOutput caps each of stdout and stderr in bytes; zero means DefaultMaxOutput. Output past the
	// cap is discarded (the child is not blocked) and reported through Result.*Truncated.
	MaxOutput int64
}

// Result is what a run produced. Apart from Duration, it is a pure function of
// the command's behaviour: output is returned byte for byte, never reordered.
type Result struct {
	Status Status
	// ExitCode is the exit status, or -1 when the command did not exit normally.
	ExitCode int
	Stdout   []byte
	Stderr   []byte
	// StdoutTruncated and StderrTruncated report output beyond MaxOutput.
	StdoutTruncated bool
	StderrTruncated bool
	// Timeout is the effective timeout that applied.
	Timeout  time.Duration
	Duration time.Duration
	// Err is the underlying error for every status other than StatusOK.
	Err error
}

// Run executes spec and returns its outcome. It never panics and never returns
// an error: failures are a Status. ctx cancellation is reported as StatusError.
func Run(ctx context.Context, spec Spec) Result {
	res := Result{ExitCode: -1}
	if len(spec.Argv) == 0 || strings.TrimSpace(spec.Argv[0]) == "" {
		res.Status, res.Err = StatusError, errors.New("empty command")
		return res
	}
	res.Timeout = EffectiveTimeout(spec.Timeout)
	path, err := resolve(spec.Argv[0], spec.Dir)
	if err != nil {
		res.Status, res.Err = StatusUnavailable, err
		return res
	}

	tctx, cancel := context.WithTimeout(ctx, res.Timeout)
	defer cancel()
	cmd := exec.CommandContext(tctx, path, spec.Argv[1:]...) //nolint:gosec // the caller supplies the argv; no shell is involved
	cmd.Dir = spec.Dir
	if !spec.InheritEnv {
		cmd.Env = append([]string{}, spec.Env...)
	}
	if spec.Stdin != nil {
		cmd.Stdin = bytes.NewReader(spec.Stdin)
	}
	limit := spec.MaxOutput
	if limit <= 0 {
		limit = DefaultMaxOutput
	}
	stdout, stderr := &capBuffer{limit: limit}, &capBuffer{limit: limit}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	tree := configure(cmd)
	defer tree.close()
	cmd.WaitDelay = killGrace

	start := time.Now()
	runErr := cmd.Start()
	if runErr == nil {
		tree.attach(cmd)
		runErr = cmd.Wait()
		// The command is done, but a daemonised helper may still be alive; no
		// scanner or verifier gets to leave processes behind.
		tree.kill(cmd)
	}
	res.Duration = time.Since(start)
	res.Stdout, res.StdoutTruncated = stdout.bytes()
	res.Stderr, res.StderrTruncated = stderr.bytes()

	switch {
	case runErr == nil:
		res.Status, res.ExitCode = StatusOK, 0
	case errors.Is(runErr, exec.ErrWaitDelay) && cmd.ProcessState != nil && !errors.Is(tctx.Err(), context.DeadlineExceeded):
		// The command exited but a helper held the pipes open past WaitDelay. The exit status is
		// what counts; the output captured so far is kept.
		if cmd.ProcessState.Success() {
			res.Status, res.ExitCode = StatusOK, 0
		} else {
			res.Status, res.ExitCode, res.Err = StatusExit, cmd.ProcessState.ExitCode(), runErr
		}
	case errors.Is(tctx.Err(), context.DeadlineExceeded):
		res.Status, res.Err = StatusTimeout, runErr
	case ctx.Err() != nil:
		res.Status, res.Err = StatusError, ctx.Err()
	default:
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			res.Status, res.ExitCode, res.Err = StatusExit, exitErr.ExitCode(), runErr
		} else {
			res.Status, res.Err = StatusError, runErr
		}
	}
	return res
}

// EffectiveTimeout applies the default and the maximum to a requested timeout.
func EffectiveTimeout(d time.Duration) time.Duration {
	switch {
	case d <= 0:
		return DefaultTimeout
	case d > MaxTimeout:
		return MaxTimeout
	}
	return d
}

// LookPath finds name on PATH. A binary found only through a relative PATH
// entry is reported as an error (exec.ErrDot), not returned. It exists so
// library packages that must know whether a tool is installed do not import
// os/exec themselves.
func LookPath(name string) (string, error) {
	return exec.LookPath(name) //nolint:wrapcheck // the message already names the binary
}

// resolve finds the executable the way a shell would, but reports a missing or
// non-executable file as an error up front so the caller can tell "not
// installed" from "ran and failed".
func resolve(name, dir string) (string, error) {
	if !strings.ContainsAny(name, `/\`) {
		p, err := exec.LookPath(name)
		if err != nil {
			// exec.ErrDot means the binary was found only through a relative PATH
			// entry, whose meaning depends on the current directory: not installed.
			return "", err //nolint:wrapcheck // the message already names the binary
		}
		return p, nil
	}
	p := name
	if !filepath.IsAbs(p) {
		base := dir
		if base == "" {
			base = "."
		}
		abs, err := filepath.Abs(filepath.Join(base, p))
		if err != nil {
			return "", err //nolint:wrapcheck // the message already names the path
		}
		p = abs
	}
	info, err := os.Stat(p)
	if err != nil {
		return "", err //nolint:wrapcheck // the message already names the path
	}
	if info.IsDir() {
		return "", &os.PathError{Op: "exec", Path: p, Err: errors.New("is a directory")}
	}
	if info.Mode()&0o111 == 0 && !isWindows() {
		return "", &os.PathError{Op: "exec", Path: p, Err: os.ErrPermission}
	}
	return p, nil
}

// capBuffer keeps at most limit bytes and silently drops the rest while still
// reporting every byte as written, so the child never blocks on a full pipe.
type capBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int64
	truncated bool
}

func (c *capBuffer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	room := c.limit - int64(c.buf.Len())
	if room <= 0 {
		c.truncated = c.truncated || len(p) > 0
		return len(p), nil
	}
	if int64(len(p)) > room {
		c.buf.Write(p[:room])
		c.truncated = true
		return len(p), nil
	}
	c.buf.Write(p)
	return len(p), nil
}

func (c *capBuffer) bytes() ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.buf.Bytes()...), c.truncated
}
