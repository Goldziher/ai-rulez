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

// probeTimeout bounds the handshake: it carries no work, so a runner that needs
// longer than this to answer is not answering.
const probeTimeout = time.Minute

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

	stdout, err := r.invoke(ctx, req, body, timeout)
	if err != nil {
		return nil, err
	}
	var resp Response
	if err := json.Unmarshal(stdout, &resp); err != nil {
		return nil, fmt.Errorf("runner command printed invalid JSON: %w", err)
	}
	if err := resp.Validate(req); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Handshake implements Handshaker: it sends the probe request (mode
// "capabilities", no skill, no cases) to the command and reads what the command
// declares. A command that does not know the probe answers with no capabilities
// (or fails), so it is never sent an activation request.
func (r *CommandRunner) Handshake(ctx context.Context) (Declaration, error) {
	if strings.TrimSpace(r.Command) == "" {
		return Declaration{}, fmt.Errorf("the command runner needs --runner-command")
	}
	req := &Request{Version: ProtocolVersion, Mode: ModeCapabilities}
	body, err := json.Marshal(req)
	if err != nil {
		return Declaration{}, fmt.Errorf("encode request: %w", err)
	}
	timeout := probeTimeout
	if r.Timeout > 0 && r.Timeout < timeout {
		timeout = r.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout, err := r.invoke(ctx, req, body, timeout)
	if err != nil {
		return Declaration{}, err
	}
	var resp Response
	if err := json.Unmarshal(stdout, &resp); err != nil {
		return Declaration{}, fmt.Errorf("runner command printed invalid JSON to the capabilities probe: %w", err)
	}
	if err := resp.ValidateProbe(); err != nil {
		return Declaration{}, err
	}
	return Declaration{Capabilities: resp.Capabilities, Surfaces: resp.Surfaces}, nil
}

// invoke starts the command through r.Runner (the real process runner when nil):
// one place owns process groups, output caps and the timeout kill. It returns the
// command's standard output.
func (r *CommandRunner) invoke(ctx context.Context, req *Request, body []byte, timeout time.Duration) ([]byte, error) {
	argv := []string{"sh", "-c", r.Command}
	if runtime.GOOS == "windows" {
		argv = []string{"cmd", "/C", r.Command}
	}
	res := runner.Or(r.Runner).Run(ctx, runner.Spec{
		Argv: argv, Stdin: body, Timeout: timeout, MaxOutput: maxResponseBytes,
		Env: append(runner.HostEnv(), fmt.Sprintf("AI_RULEZ_EVAL_PROTOCOL=%d", ProtocolVersion), "AI_RULEZ_EVAL_SKILL="+req.Skill.ID, "AI_RULEZ_EVAL_MODE="+req.Mode),
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
	return res.Stdout, nil
}

// Runner names.
const (
	RunnerClaudePluginEval = "claude-plugin-eval"
	RunnerCommand          = "command"
	// RunnerClaudeNative and RunnerCodexNative drive the harness's own CLI for the
	// native activation surface.
	RunnerClaudeNative = "claude-native"
	RunnerCodexNative  = "codex-native"
)
