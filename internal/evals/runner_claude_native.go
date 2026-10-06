package evals

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// claudeNativePlugin is the name of the throwaway plugin that carries the
// installed set; Claude Code namespaces its skills as "<plugin>:<skill>".
const claudeNativePlugin = "ai-rulez-activation"

// defaultClaudeRunTimeout bounds one activation run: the decision is in the first
// turn, so a run that takes minutes is stuck.
const defaultClaudeRunTimeout = 3 * time.Minute

// ClaudeNative is the multi-skill Claude Code adapter of the native activation
// surface. It writes every skill of the request into one throwaway plugin, and for
// each prompt and each repetition starts `claude -p` in an empty directory with
// that plugin, one turn, and only the Skill tool; the skills that loaded are read
// from the stream-json transcript (a Skill tool_use names the skill). It declares
// the activation capability and the native surface, and nothing else: it does not
// run full cases (use claude-plugin-eval for those).
//
// The adapter loads no user, project or local settings (--setting-sources ""), so
// the skills in the user's own Claude Code configuration do not compete with the
// installed set; the harness's built-in skills do, and so do any enabled plugins.
type ClaudeNative struct {
	// Bin is the claude executable. Default "claude".
	Bin string
	// Timeout bounds one run. Default 3 minutes.
	Timeout time.Duration
	// Concurrency is how many runs are in flight. Default 4.
	Concurrency int
	// ExtraArgs are appended to every claude command line.
	ExtraArgs []string
	// KeepDir, when set, is where the plugin is built and kept.
	KeepDir string
	Stderr  io.Writer
	// Exec runs one claude process; tests replace it. Nil goes through Runner.
	Exec func(ctx context.Context, bin string, args []string, dir string, stdin []byte) (stdout, stderr []byte, err error)
	// Runner starts the process when Exec is nil; nil runs a real process.
	Runner runner.Runner
}

// Name implements Runner.
func (*ClaudeNative) Name() string { return RunnerClaudeNative }

// Capabilities implements CapabilityReporter.
func (*ClaudeNative) Capabilities() []string { return []string{CapabilityActivation} }

// Surfaces implements SurfaceReporter.
func (*ClaudeNative) Surfaces() []string { return []string{SurfaceNative} }

// Fingerprint implements Fingerprinter.
func (r *ClaudeNative) Fingerprint() string {
	bin := r.bin()
	return fmt.Sprintf("bin=%s binfile=%s args=%q", bin, executableStamp(bin), r.ExtraArgs)
}

func (r *ClaudeNative) bin() string {
	if r.Bin == "" {
		return "claude"
	}
	return r.Bin
}

// Run implements Runner.
func (r *ClaudeNative) Run(ctx context.Context, req *Request) (*Response, error) {
	if req.Harness != "" && req.Harness != "claude" {
		return nil, fmt.Errorf("the %s runner only drives the claude harness, not %q", RunnerClaudeNative, req.Harness)
	}
	dir := r.KeepDir
	if dir == "" {
		tmp, err := os.MkdirTemp("", "ai-rulez-activation-*")
		if err != nil {
			return nil, fmt.Errorf("create temp plugin: %w", err)
		}
		defer os.RemoveAll(tmp) //nolint:errcheck // throwaway directory
		dir = tmp
	}
	pluginDir, workDir := filepath.Join(dir, "plugin"), filepath.Join(dir, "work")
	if err := os.RemoveAll(pluginDir); err != nil {
		return nil, fmt.Errorf("clear stale plugin: %w", err)
	}
	ids, err := buildClaudeActivationPlugin(pluginDir, req)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(workDir, 0o750); err != nil {
		return nil, fmt.Errorf("create work directory: %w", err)
	}
	args := r.args(pluginDir, req)
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultClaudeRunTimeout
	}
	exec := r.Exec
	if exec == nil {
		exec = r.execThrough(timeout)
	}
	return runActivationRuns(ctx, req, r.Concurrency, func(ctx context.Context, c *Case) activationOutcome {
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		stdout, stderr, err := exec(runCtx, r.bin(), args, workDir, []byte(c.Prompt))
		if r.Stderr != nil && len(stderr) > 0 {
			_, _ = r.Stderr.Write(stderr) //nolint:errcheck // best-effort forwarding
		}
		return parseClaudeActivation(stdout, stderr, err, ids)
	})
}

// args is the claude command line of one run. The prompt goes on stdin, so a
// prompt that starts with a dash is not an option.
func (r *ClaudeNative) args(pluginDir string, req *Request) []string {
	turns := req.MaxTurns
	if turns < 1 {
		turns = ActivationMaxTurns
	}
	args := []string{
		"-p", "--output-format", "stream-json", "--verbose", "--max-turns", strconv.Itoa(turns),
		"--no-session-persistence", "--setting-sources", "", "--permission-mode", "dontAsk",
		"--plugin-dir", pluginDir, "--tools", "Skill", "--allowedTools", "Skill",
	}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	return append(args, r.ExtraArgs...)
}

func (r *ClaudeNative) execThrough(timeout time.Duration) func(ctx context.Context, bin string, args []string, dir string, stdin []byte) ([]byte, []byte, error) {
	return func(ctx context.Context, bin string, args []string, dir string, stdin []byte) ([]byte, []byte, error) {
		res := runner.Or(r.Runner).Run(ctx, runner.Spec{
			Argv: append([]string{bin}, args...), Dir: dir, InheritEnv: true, Stdin: stdin,
			Timeout: timeout, MaxOutput: maxToolOutputBytes,
		})
		// A run that stops at its turn limit exits non-zero by design; the transcript
		// decides, not the exit status.
		if res.Status == runner.StatusOK || res.Status == runner.StatusExit {
			return res.Stdout, res.Stderr, nil
		}
		return res.Stdout, res.Stderr, res.Err
	}
}

// buildClaudeActivationPlugin writes the installed set as one plugin and returns
// the ids of its skills.
func buildClaudeActivationPlugin(dir string, req *Request) (map[string]bool, error) {
	manifest := fmt.Sprintf("{\n  \"name\": %q,\n  \"version\": \"0.0.0\",\n  \"description\": \"Throwaway plugin built by ai-rulez eval run --mode activation\"\n}\n", claudeNativePlugin)
	if err := writeFile(filepath.Join(dir, ".claude-plugin", "plugin.json"), manifest); err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	skills := req.Skills
	if len(skills) == 0 {
		skills = []SkillRef{req.Skill}
	}
	for i := range skills {
		s := &skills[i]
		if s.ID == "" || s.Dir == "" {
			return nil, fmt.Errorf("activation request has a skill without id or dir")
		}
		if err := copyTree(s.Dir, filepath.Join(dir, "skills", s.ID), "evals"); err != nil {
			return nil, fmt.Errorf("copy skill %q: %w", s.ID, err)
		}
		ids[s.ID] = true
	}
	return ids, nil
}

// claudeStreamLine is the subset of a stream-json line the adapter reads.
type claudeStreamLine struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	IsError bool   `json:"is_error"`
	Result  string `json:"result"`
	Message struct {
		Content []struct {
			Type  string `json:"type"`
			Name  string `json:"name"`
			Input struct {
				Skill string `json:"skill"`
			} `json:"input"`
		} `json:"content"`
	} `json:"message"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	Usage        struct {
		InputTokens         int `json:"input_tokens"`
		CacheCreationTokens int `json:"cache_creation_input_tokens"`
		CacheReadTokens     int `json:"cache_read_input_tokens"`
		OutputTokens        int `json:"output_tokens"`
	} `json:"usage"`
}

// maxStreamLine bounds one transcript line (a tool result can be large).
const maxStreamLine = 16 << 20

// parseClaudeActivation reads one run's stream-json transcript: the skills of ids
// that were called through the Skill tool, and the usage of the closing result
// line. A run without a result line failed.
func parseClaudeActivation(stdout, stderr []byte, runErr error, ids map[string]bool) activationOutcome {
	out := activationOutcome{fired: map[string]bool{}}
	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	scanner.Buffer(make([]byte, 0, 64<<10), maxStreamLine)
	sawResult := false
	for scanner.Scan() {
		var line claudeStreamLine
		if json.Unmarshal(scanner.Bytes(), &line) != nil {
			continue
		}
		switch line.Type {
		case "assistant":
			for _, block := range line.Message.Content {
				if block.Type == "tool_use" && block.Name == "Skill" {
					if id := skillIDOf(block.Input.Skill, ids); id != "" {
						out.fired[id] = true
					}
				}
			}
		case "result":
			sawResult = true
			out.costUSD = line.TotalCostUSD
			out.input = line.Usage.InputTokens + line.Usage.CacheCreationTokens + line.Usage.CacheReadTokens
			out.output = line.Usage.OutputTokens
			// error_max_turns is the expected end of a one-turn run; any other error
			// result (authentication, rate limit) is a failed run.
			if line.IsError && line.Subtype != "error_max_turns" {
				out.err = "claude reported an error: " + firstLine(line.Result, line.Subtype)
			}
		}
	}
	switch {
	case out.err != "":
	case !sawResult:
		out.err = "claude printed no result: " + firstLine(string(stderr), errText(runErr))
	}
	return out
}

// skillIDOf maps a Skill tool argument ("plugin:skill" or "skill") to an installed id.
func skillIDOf(arg string, ids map[string]bool) string {
	arg = strings.TrimSpace(arg)
	if ids[arg] {
		return arg
	}
	if i := strings.LastIndex(arg, ":"); i >= 0 && ids[arg[i+1:]] {
		return arg[i+1:]
	}
	return ""
}

func errText(err error) string {
	if err == nil {
		return "no output"
	}
	return err.Error()
}

// firstLine is the first non-empty line of s (bounded), or fallback.
func firstLine(s, fallback string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			if len(line) > 200 {
				line = line[:200]
			}
			return line
		}
	}
	return fallback
}
