package verifiers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/verifiers/vspec"
)

const (
	passFilesArgs   = "args"
	passFilesStdin0 = "stdin0"

	// defaultCommandTimeout applies when a command sets no timeout_s.
	defaultCommandTimeout = 2 * time.Minute
	// defaultMaxTimeoutS is [verifiers_settings] max_timeout_s when unset.
	defaultMaxTimeoutS = 300
	// maxCommandOutput caps each captured stream of a command.
	maxCommandOutput = 64 << 10
	// maxArgFiles and maxArgBytes bound pass_files = "args" so the command line stays
	// below the platform limit; use stdin0 beyond them.
	maxArgFiles = 500
	maxArgBytes = 64 << 10
	// maxCommandTimeoutS is the longest timeout_s a declaration may ask for.
	maxCommandTimeoutS = 900
	// maxStatus is the largest process exit status.
	maxStatus = 255
)

// codedError is an evaluation error that carries its AR9H code and status: a
// command that was refused or did not run (AR9H3, status error) or an LLM
// verifier that was skipped (AR9H4, status skipped).
type codedError struct {
	code   string
	status Status
	msg    string
	// stop ends the verifier's remaining model calls (an LLM skip only).
	stop bool
}

func (e *codedError) Error() string { return e.msg }

func execErrorf(format string, args ...any) error {
	return &codedError{code: CodeVerifierCommand, status: StatusError, msg: fmt.Sprintf(format, args...)}
}

// settings returns [verifiers_settings] (the zero value when absent).
func (e *Env) settings() config.VerifiersSettings {
	if e.Cfg != nil && e.Cfg.VerifiersSettings != nil {
		return *e.Cfg.VerifiersSettings
	}
	return config.VerifiersSettings{}
}

func (e *Env) commandRunner() runner.Runner {
	if e.opts.Runner != nil {
		return e.opts.Runner
	}
	return runner.Exec{}
}

// commandEnv is the environment a command gets: the runner's base allowlist, CI
// and the names of [verifiers_settings] command_env, nothing else.
func (e *Env) commandEnv() []string {
	parent := e.opts.Environ
	if parent == nil {
		parent = runner.HostEnv()
	}
	pass := append([]string{"CI"}, e.settings().CommandEnv...)
	return runner.ScrubEnv(parent, pass, nil)
}

// execAllowed refuses a command predicate unless --allow-exec was given and, for
// a verifier that arrived through an include, the include is trusted.
func (c *evalCtx) execAllowed() error {
	if !c.env.opts.AllowExec {
		return execErrorf("the command predicate runs a program: pass --allow-exec (or set AI_RULEZ_VERIFIERS_ALLOW_EXEC=1) to run it")
	}
	if c.spec.origin != "" && !importTrusted(c.env.settings(), c.spec.origin) {
		return execErrorf("verifier %s comes from include %q, which may not run commands (add it to [verifiers_settings] trust_exec_from)", c.spec.ID, c.spec.origin)
	}
	return nil
}

func (c *evalCtx) evalCommand(ctx context.Context, p *vspec.CommandPred) (evalOut, error) {
	if err := c.execAllowed(); err != nil {
		return evalOut{}, err
	}
	argv, stdin, err := c.commandInput(p)
	if err != nil {
		return evalOut{}, err
	}
	res := c.env.commandRunner().Run(ctx, runner.Spec{
		Argv: argv, Dir: c.env.Root, Env: c.env.commandEnv(), Stdin: stdin, Timeout: c.commandTimeout(p), MaxOutput: maxCommandOutput,
	})
	if res.StdoutTruncated || res.StderrTruncated {
		c.note("%s: output was truncated to %d KiB", argv[0], maxCommandOutput>>10)
	}
	if err := commandRunError(argv[0], &res); err != nil {
		return evalOut{}, err
	}
	want := 0
	if p.ExpectExit != nil {
		want = *p.ExpectExit
	}
	if res.ExitCode == want {
		return evalOut{pass: true}, nil
	}
	f := Finding{Message: "`" + argv[0] + "`" + fmt.Sprintf(" exited %d, expected %d", res.ExitCode, want), Match: outputExcerpt(res)}
	return evalOut{findings: []Finding{f}}, nil
}

// commandInput builds the argument vector and standard input of the command,
// passing the scoped files as arguments or as NUL-separated standard input.
func (c *evalCtx) commandInput(p *vspec.CommandPred) (argv []string, stdin []byte, err error) {
	argv = append([]string(nil), p.Argv...)
	switch p.PassFiles {
	case passFilesArgs:
		total := 0
		for _, f := range c.scoped {
			total += len(f) + 1
		}
		if len(c.scoped) > maxArgFiles || total > maxArgBytes {
			return nil, nil, execErrorf("%d changed files are too many to pass as arguments: use pass_files = \"stdin0\"", len(c.scoped))
		}
		for _, f := range c.scoped {
			argv = append(argv, argPath(f))
		}
	case passFilesStdin0:
		stdin = []byte(strings.Join(c.scoped, "\x00"))
		if len(c.scoped) > 0 {
			stdin = append(stdin, 0)
		}
	}
	if stdin == nil {
		stdin = []byte{}
	}
	return argv, stdin, nil
}

// commandTimeout is the predicate's timeout capped at the configured maximum.
func (c *evalCtx) commandTimeout(p *vspec.CommandPred) time.Duration {
	timeout := defaultCommandTimeout
	if p.TimeoutS > 0 {
		timeout = time.Duration(p.TimeoutS) * time.Second
	}
	maxT := c.env.settings().MaxTimeoutS
	if maxT <= 0 {
		maxT = defaultMaxTimeoutS
	}
	if limit := time.Duration(maxT) * time.Second; timeout > limit {
		timeout = limit
	}
	return timeout
}

// commandRunError turns a run that did not complete into an execution error.
func commandRunError(name string, res *runner.Result) error {
	switch res.Status {
	case runner.StatusOK, runner.StatusExit:
		return nil
	case runner.StatusTimeout:
		return execErrorf("%s timed out after %s", name, res.Timeout)
	case runner.StatusUnavailable:
		return execErrorf("%s could not be started: %s", name, errText(res.Err))
	default:
		return execErrorf("%s did not run: %s", name, errText(res.Err))
	}
}

// argPath makes a repo-relative path safe as a command-line argument: a name that
// starts with "-" would otherwise be read as an option by the program (a file
// named --require=... in a pull request), so it gets a "./" prefix.
func argPath(f string) string {
	if strings.HasPrefix(f, "-") {
		return "./" + f
	}
	return f
}

func errText(err error) string {
	if err == nil {
		return "unknown error"
	}
	var denied *runner.DeniedError
	if errors.As(err, &denied) {
		return "running commands is not allowed here"
	}
	return llm.RedactSecrets(err.Error())
}

// outputExcerpt is the first non-empty line of stderr (else stdout) of a
// failed command, cut at 120 characters with secrets masked.
func outputExcerpt(res runner.Result) string {
	for _, stream := range [][]byte{res.Stderr, res.Stdout} {
		for _, line := range bytes.Split(stream, []byte{'\n'}) {
			if text := strings.TrimSpace(string(line)); text != "" {
				return excerpt([]byte(text), 0, len(text))
			}
		}
	}
	return ""
}

// importTrusted reports whether verifiers imported from the include may run commands.
func importTrusted(s config.VerifiersSettings, include string) bool {
	for _, n := range s.TrustExecFrom {
		if n == include {
			return true
		}
	}
	return false
}
