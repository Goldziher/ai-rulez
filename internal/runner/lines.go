package runner

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"time"
)

// LineFunc receives one line of a command's standard output (without its newline)
// and reports whether the command may stop now. The slice is only valid during the call.
type LineFunc func(line []byte) (stop bool)

// LineRunner is a Runner that can also hand a command's standard output to the
// caller line by line while it runs and end the command early. The eval adapters
// use it to stop a harness at its first decision instead of paying for the rest of
// its run.
type LineRunner interface {
	RunLines(ctx context.Context, spec Spec, onLine LineFunc) Result
}

// maxLine bounds one streamed line; a longer one is cut (the rest is dropped).
const maxLine = 16 << 20

// RunLines implements LineRunner for Exec.
func (Exec) RunLines(ctx context.Context, spec Spec, onLine LineFunc) Result { //nolint:gocyclo // one linear start, stream, stop and classify sequence
	res := Result{ExitCode: -1}
	if len(spec.Argv) == 0 {
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
	stderr := &capBuffer{limit: limit}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		res.Status, res.Err = StatusError, err
		return res
	}
	tree := configure(cmd)
	defer tree.close()
	cmd.WaitDelay = killGrace

	start := time.Now()
	if err := cmd.Start(); err != nil {
		res.Status, res.Err = StatusError, err
		return res
	}
	tree.attach(cmd)
	stopped := false
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLine)
	for scanner.Scan() {
		if onLine(scanner.Bytes()) {
			stopped = true
			break
		}
	}
	if stopped {
		// The caller has what it wanted: end the whole process tree now. No drain:
		// a child forked while the signal was sent can escape it and keep the pipe
		// open; Wait closes the pipe once the command itself has exited, and the
		// kill after Wait reaches the straggler, which is still in the group.
		tree.kill(cmd)
	} else {
		_, _ = io.Copy(io.Discard, stdout) //nolint:errcheck // drain so Wait cannot block on the pipe
	}
	waitErr := cmd.Wait()
	tree.kill(cmd) // a helper that outlived the command must not outlive the run
	res.Duration = time.Since(start)
	res.Stderr, res.StderrTruncated = stderr.bytes()

	switch {
	case stopped:
		res.Status, res.ExitCode = StatusOK, 0
	case errors.Is(tctx.Err(), context.DeadlineExceeded):
		res.Status, res.Err = StatusTimeout, waitErr
	case ctx.Err() != nil:
		res.Status, res.Err = StatusError, ctx.Err()
	case waitErr == nil:
		res.Status, res.ExitCode = StatusOK, 0
	default:
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			res.Status, res.ExitCode, res.Err = StatusExit, exitErr.ExitCode(), waitErr
		} else {
			res.Status, res.Err = StatusError, waitErr
		}
	}
	return res
}

// RunLines implements LineRunner for Deny: nothing is started.
func (d Deny) RunLines(ctx context.Context, spec Spec, _ LineFunc) Result { return d.Run(ctx, spec) }

// AsLineRunner returns r as a LineRunner: the real process runner for nil, r itself
// when it can stream, and ok false otherwise.
func AsLineRunner(r Runner) (LineRunner, bool) {
	if r == nil {
		return Exec{}, true
	}
	lr, ok := r.(LineRunner)
	return lr, ok
}
