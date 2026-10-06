package evals

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	return r.runThrough(ctx, req, body, timeout)
}

// runThrough starts the command through r.Runner (the real process runner when nil):
// one place owns process groups, output caps and the timeout kill.
func (r *CommandRunner) runThrough(ctx context.Context, req *Request, body []byte, timeout time.Duration) (*Response, error) {
	argv := []string{"sh", "-c", r.Command}
	if runtime.GOOS == "windows" {
		argv = []string{"cmd", "/C", r.Command}
	}
	res := runner.Or(r.Runner).Run(ctx, runner.Spec{
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

// Runner names.
const (
	RunnerClaudePluginEval = "claude-plugin-eval"
	RunnerCommand          = "command"
)
