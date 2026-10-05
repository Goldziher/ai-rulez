package evals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"
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
}

// Name implements Runner.
func (*CommandRunner) Name() string { return RunnerCommand }

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
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", r.Command) //nolint:gosec // the user configured this command
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", r.Command) //nolint:gosec // the user configured this command
	}
	cmd.Stdin = bytes.NewReader(body)
	var stdout bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, n: maxResponseBytes}
	cmd.Stderr = r.Stderr
	cmd.Env = append(cmd.Environ(), fmt.Sprintf("AI_RULEZ_EVAL_PROTOCOL=%d", ProtocolVersion), "AI_RULEZ_EVAL_SKILL="+req.Skill.ID)
	if err := cmd.Run(); err != nil {
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
