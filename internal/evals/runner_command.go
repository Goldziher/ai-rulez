package evals

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// maxResponseBytes bounds a runner's answer.
const maxResponseBytes = 64 << 20

// CommandRunner pipes the request JSON to a user-supplied command and reads the
// response JSON from its standard output. The command is run through the shell;
// it is the integration point for other harnesses and in-house infrastructure,
// and it, not ai-rulez, decides where a prompt goes.
type CommandRunner struct {
	// Command is the shell command line.
	Command string
	// Timeout bounds one invocation (one skill). Default 30 minutes.
	Timeout time.Duration
	// Stderr receives the command's standard error. Nil discards it.
	Stderr io.Writer
	// Runner starts the command; nil runs it with os/exec. With a Runner the
	// request goes through it (and so can be denied or recorded).
	Runner runner.Runner
}

// Name implements Runner.
func (*CommandRunner) Name() string { return RunnerCommand }

// Fingerprint implements Fingerprinter: a different command is a different runner.
func (r *CommandRunner) Fingerprint() string {
	return "command=" + r.Command + " program=" + commandProgramStamp(r.Command)
}

// Run implements Runner.
func (r *CommandRunner) Run(ctx context.Context, req *Request) (*Response, error) {
	if strings.TrimSpace(r.Command) == "" {
		return nil, fmt.Errorf("the command runner needs --runner-command")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultRunnerTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if r.Runner != nil {
		return r.runThrough(ctx, req, body, timeout)
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", r.Command) //nolint:gosec // the user configured this command
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", r.Command) //nolint:gosec // the user configured this command
	}
	killTreeOnCancel(cmd)
	cmd.Stdin = bytes.NewReader(body)
	var stdout bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, n: maxResponseBytes}
	cmd.Stderr = r.Stderr
	cmd.Env = append(cmd.Environ(), fmt.Sprintf("AI_RULEZ_EVAL_PROTOCOL=%d", ProtocolVersion), "AI_RULEZ_EVAL_SKILL="+req.Skill.ID)
	if err := runTree(cmd); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("runner command timed out after %s (raise --timeout)", timeout)
		}
		return nil, fmt.Errorf("runner command failed: %w", err)
	}
	var resp Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("runner command printed invalid JSON: %w", err)
	}
	if err := resp.Validate(req); err != nil {
		return nil, err
	}
	return &resp, nil
}

// runThrough is Run with the command started by r.Runner.
func (r *CommandRunner) runThrough(ctx context.Context, req *Request, body []byte, timeout time.Duration) (*Response, error) {
	argv := []string{"sh", "-c", r.Command}
	if runtime.GOOS == "windows" {
		argv = []string{"cmd", "/C", r.Command}
	}
	res := r.Runner.Run(ctx, runner.Spec{
		Argv: argv, Stdin: body, Timeout: timeout, MaxOutput: maxResponseBytes,
		Env: append(runner.HostEnv(), fmt.Sprintf("AI_RULEZ_EVAL_PROTOCOL=%d", ProtocolVersion), "AI_RULEZ_EVAL_SKILL="+req.Skill.ID),
	})
	if r.Stderr != nil {
		_, _ = r.Stderr.Write(res.Stderr) //nolint:errcheck // best-effort forwarding
	}
	switch {
	case res.Status == runner.StatusTimeout || errors.Is(ctx.Err(), context.DeadlineExceeded):
		return nil, fmt.Errorf("runner command timed out after %s (raise --timeout)", timeout)
	case res.Status != runner.StatusOK:
		return nil, fmt.Errorf("runner command failed: %w", cmp.Or(res.Err, fmt.Errorf("exit status %d", res.ExitCode)))
	case res.StdoutTruncated:
		return nil, fmt.Errorf("runner output exceeds %d bytes", maxResponseBytes)
	}
	var resp Response
	if err := json.Unmarshal(res.Stdout, &resp); err != nil {
		return nil, fmt.Errorf("runner command printed invalid JSON: %w", err)
	}
	if err := resp.Validate(req); err != nil {
		return nil, err
	}
	return &resp, nil
}

type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if len(p) > l.n {
		return 0, fmt.Errorf("runner output exceeds %d bytes", maxResponseBytes)
	}
	l.n -= len(p)
	return l.w.Write(p)
}

// Runner names.
const (
	RunnerClaudePluginEval = "claude-plugin-eval"
	RunnerCommand          = "command"
)
