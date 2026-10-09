package improve

import (
	"context"
	"os"
	"slices"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// gitLocalEnv returns the variables a local git command (worktree, add, commit) keeps besides the base set: where
// its configuration lives and who the commit is by. No credential is among them.
func gitLocalEnv() []string {
	return []string{
		"XDG_CONFIG_HOME", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM", "GIT_CONFIG_NOSYSTEM", "GIT_EXEC_PATH",
		"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_AUTHOR_DATE",
		"GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "GIT_COMMITTER_DATE",
	}
}

// gitPushEnv returns the extra variables the push keeps: the transports and credential helpers git needs to reach
// the remote. Tokens of other tools (a model key, gh's token) never reach git.
func gitPushEnv() []string {
	return []string{"SSH_AUTH_SOCK", "SSH_ASKPASS", "GIT_ASKPASS", "GIT_SSH", "GIT_SSH_COMMAND", "GCM_INTERACTIVE"}
}

// hardenedGit returns g with its processes confined: every git command gets a scrubbed environment (host is
// the process environment it is scrubbed from), runs no hook, and only the push gets the variables that carry
// transport credentials. Nothing goes through a shell: the runner starts git with an argument vector.
func hardenedGit(g gitutil.Git, host []string) gitutil.Git {
	inner := runner.Or(g.Runner)
	g.Runner = runner.Func(func(ctx context.Context, spec runner.Spec) runner.Result {
		pass := gitLocalEnv()
		if slices.Contains(spec.Argv, "push") {
			pass = append(gitLocalEnv(), gitPushEnv()...)
		}
		spec.Env = runner.ScrubEnv(host, pass, []string{"GIT_TERMINAL_PROMPT=0"})
		if len(spec.Argv) > 0 {
			spec.Argv = append([]string{spec.Argv[0], "-c", "core.hooksPath=" + os.DevNull, "-c", "core.fsmonitor=false"}, spec.Argv[1:]...)
		}
		return inner.Run(ctx, spec)
	})
	return g
}
