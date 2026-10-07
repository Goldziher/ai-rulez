package gitutil

import (
	"context"
	"fmt"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// Git answers repository questions by running git through a runner.Runner. The
// zero value runs real processes (runner.Exec), so Git{} is what the package
// functions in free.go use; a service builds one with New and a Deny or Fake
// runner to control every command.
type Git struct {
	// Runner starts the git processes; nil means runner.Exec.
	Runner runner.Runner
	// Log receives the reports of the ignore-file helpers; nil is the CLI's logger.
	Log logger.Logger
}

// WithLog returns g reporting to log.
func (g Git) WithLog(log logger.Logger) Git {
	g.Log = log
	return g
}

// New returns a Git that runs git through r (nil: runner.Exec).
func New(r runner.Runner) Git { return Git{Runner: r} }

func (g Git) runner() runner.Runner { return runner.Or(g.Runner) }

// Exec runs git with the given environment (nil: Env(nil), the process
// environment without repository selection) and returns the raw outcome. It is
// the seam for callers that need the exit status, stderr or a custom
// environment (include fetches); a non-empty dir becomes `-C dir`. The command
// may run up to runner.MaxTimeout, so a clone of a large repository is not cut
// short by the runner's default.
func (g Git) Exec(ctx context.Context, dir string, env []string, args ...string) runner.Result {
	if env == nil {
		env = Env(nil)
	}
	return g.runner().Run(ctx, runner.Spec{
		Argv:    append([]string{"git"}, gitArgs(dir, args)...),
		Env:     env,
		Timeout: runner.MaxTimeout,
	})
}

// ResultErr is the error of a failed run: nil for StatusOK, else the runner's
// error (an *exec.ExitError for a non-zero exit).
func ResultErr(res runner.Result) error {
	if res.Status == runner.StatusOK {
		return nil
	}
	if res.Err != nil {
		return res.Err
	}
	return fmt.Errorf("git exited with status %d", res.ExitCode)
}
