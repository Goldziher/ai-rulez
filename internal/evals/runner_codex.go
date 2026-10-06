package evals

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// CodexProcess is one codex invocation: the program, its arguments, the directory
// and environment it runs in, and the prompt on its standard input.
type CodexProcess struct {
	Bin   string
	Args  []string
	Dir   string
	Env   []string
	Stdin []byte
}

// defaultCodexRunTimeout bounds one run; a run is stopped at its first commands, so
// this is a ceiling for a stuck process.
const defaultCodexRunTimeout = 3 * time.Minute

// codexCommandLimit is how many shell commands a codex run may start before the
// adapter stops it: codex loads a skill by reading its SKILL.md, usually as its
// first command, so a run that has not read one after a few commands will not.
const codexCommandLimit = 3

// CodexNative is the Codex CLI adapter of the native activation surface. It
// writes the installed set to <work>/.agents/skills/<id>/, starts `codex exec
// --json` there for each prompt and repetition, and reads which skills the model
// loaded: Codex loads a skill by reading its SKILL.md, which shows up as a shell
// command. The run is stopped (the process group is killed) as soon as a skill was
// read or after codexCommandLimit commands, so the model never gets to act on the
// task. The commands it did start run in a read-only sandbox in an empty
// directory, with HOME pointed at an empty directory (so a kubeconfig or cloud
// credentials in the real home are out of reach) and a scrubbed environment;
// CODEX_HOME stays the real one, for the login.
//
// Codex reports token usage only when a run finishes, and an early stop never does,
// so this adapter reports no tokens and no cost; --max-cost cannot be enforced
// through it.
type CodexNative struct {
	// Bin is the codex executable. Default "codex".
	Bin string
	// Timeout bounds one run. Default 3 minutes.
	Timeout time.Duration
	// Concurrency is how many runs are in flight. Default 2.
	Concurrency int
	// ExtraArgs are appended to every codex command line.
	ExtraArgs []string
	KeepDir   string
	Stderr    io.Writer
	// Start runs one codex process and returns its stdout line by line; tests replace
	// it. Nil starts a real process.
	Start func(ctx context.Context, proc CodexProcess, onLine func(line []byte) (stop bool)) error
}

// Name implements Runner.
func (*CodexNative) Name() string { return RunnerCodexNative }

// Capabilities implements CapabilityReporter.
func (*CodexNative) Capabilities() []string { return []string{CapabilityActivation} }

// Surfaces implements SurfaceReporter.
func (*CodexNative) Surfaces() []string { return []string{SurfaceNative} }

func (r *CodexNative) bin() string {
	if r.Bin == "" {
		return "codex"
	}
	return r.Bin
}

// Fingerprint implements Fingerprinter.
func (r *CodexNative) Fingerprint() string {
	return fmt.Sprintf("bin=%s binfile=%s args=%q", r.bin(), executableStamp(r.bin()), r.ExtraArgs)
}

// Run implements Runner.
func (r *CodexNative) Run(ctx context.Context, req *Request) (*Response, error) {
	if req.Harness != "" && req.Harness != "codex" {
		return nil, fmt.Errorf("the %s runner only drives the codex harness, not %q", RunnerCodexNative, req.Harness)
	}
	dir := r.KeepDir
	if dir == "" {
		tmp, err := os.MkdirTemp("", "ai-rulez-codex-*")
		if err != nil {
			return nil, fmt.Errorf("create temp directory: %w", err)
		}
		defer os.RemoveAll(tmp) //nolint:errcheck // throwaway directory
		dir = tmp
	}
	workDir, homeDir := filepath.Join(dir, "work"), filepath.Join(dir, "home")
	if err := os.RemoveAll(workDir); err != nil {
		return nil, fmt.Errorf("clear stale work directory: %w", err)
	}
	if err := os.MkdirAll(homeDir, 0o750); err != nil {
		return nil, fmt.Errorf("create home directory: %w", err)
	}
	ids, err := buildCodexSkills(workDir, req)
	if err != nil {
		return nil, err
	}
	args := r.args(workDir, req)
	env := runner.ScrubEnv(os.Environ(), nil, []string{"HOME=" + homeDir, "CODEX_HOME=" + codexHome()})
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultCodexRunTimeout
	}
	start := r.Start
	if start == nil {
		start = startProcessLines
	}
	concurrency := r.Concurrency
	if concurrency < 1 {
		concurrency = 2
	}
	return runActivationRuns(ctx, req, concurrency, func(ctx context.Context, c *Case) activationOutcome {
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		watch := &codexWatch{ids: ids}
		err := start(runCtx, CodexProcess{Bin: r.bin(), Args: args, Dir: workDir, Env: env, Stdin: []byte(c.Prompt)}, watch.line)
		return watch.outcome(err)
	})
}

func (r *CodexNative) args(workDir string, req *Request) []string {
	args := []string{"exec", "--json", "--skip-git-repo-check", "--ephemeral", "--ignore-user-config", "--ignore-rules",
		"-s", "read-only", "-C", workDir}
	if req.Model != "" {
		args = append(args, "-m", req.Model)
	}
	args = append(args, r.ExtraArgs...)
	return append(args, "-") // the prompt comes from stdin
}

// codexHome is the directory that holds the Codex login.
func codexHome() string {
	if v := os.Getenv("CODEX_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

// buildCodexSkills writes the installed set under <work>/.agents/skills.
func buildCodexSkills(workDir string, req *Request) (map[string]bool, error) {
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
		if err := copyTree(s.Dir, filepath.Join(workDir, ".agents", "skills", s.ID), "evals"); err != nil {
			return nil, fmt.Errorf("copy skill %q: %w", s.ID, err)
		}
		ids[s.ID] = true
	}
	return ids, nil
}

// codexWatch reads the event stream of one run.
type codexWatch struct {
	ids      map[string]bool
	fired    map[string]bool
	commands int
	done     bool
	sawStart bool
}

type codexEvent struct {
	Type string `json:"type"`
	Item struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	} `json:"item"`
}

// line consumes one JSONL event and reports whether the run can stop.
func (w *codexWatch) line(b []byte) bool {
	var ev codexEvent
	if json.Unmarshal(b, &ev) != nil {
		return false
	}
	switch ev.Type {
	case "turn.started", "thread.started":
		w.sawStart = true
	case "item.started":
		if ev.Item.Type != "command_execution" {
			return false
		}
		w.commands++
		for id := range w.ids {
			if codexReadsSkill(ev.Item.Command, id) {
				if w.fired == nil {
					w.fired = map[string]bool{}
				}
				w.fired[id] = true
			}
		}
		if len(w.fired) > 0 || w.commands >= codexCommandLimit {
			w.done = true
		}
	case "turn.completed":
		w.done = true
	}
	return w.done
}

// codexReadsSkill says whether a shell command reads the SKILL.md of skill id, the
// signal a Codex harness gives that it loaded the skill.
func codexReadsSkill(command, id string) bool {
	return strings.Contains(command, "/skills/"+id+"/SKILL.md") || strings.Contains(command, "skills/"+id+"/SKILL.md")
}

func (w *codexWatch) outcome(runErr error) activationOutcome {
	out := activationOutcome{fired: w.fired}
	if out.fired == nil {
		out.fired = map[string]bool{}
	}
	if !w.done && !w.sawStart {
		out.err = "codex produced no events: " + errText(runErr)
	}
	return out
}

// startProcessLines runs a process, feeds its stdout line by line to onLine and
// kills its process group as soon as onLine returns true.
func startProcessLines(ctx context.Context, proc CodexProcess, onLine func([]byte) bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	bin := proc.Bin
	cmd := exec.CommandContext(ctx, bin, proc.Args...) //nolint:gosec // the adapter's own argv; no shell involved
	cmd.Dir, cmd.Env = proc.Dir, proc.Env
	cmd.Stdin = bytes.NewReader(proc.Stdin)
	isolateProcessGroup(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("pipe codex output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", bin, err)
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64<<10), maxStreamLine)
	for scanner.Scan() {
		if onLine(scanner.Bytes()) {
			cancel() // kills the process group
			break
		}
	}
	_, _ = io.Copy(io.Discard, stdout) //nolint:errcheck // drain so Wait cannot block on the pipe
	_ = cmd.Wait()                     //nolint:errcheck // killed on purpose; the events decide
	return scanner.Err()
}
