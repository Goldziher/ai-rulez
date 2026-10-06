package gitutil

import (
	"context"
	"os/exec"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// strippedEnv are the variables that select which repository, index or object
// store git operates on. A git hook inherits them from the git process that
// started it, so a child `git` would otherwise act on the hook's repository
// whatever -C or clone destination it is given (for example `git -C <tmp> init`
// re-initializes the hook's GIT_DIR and sets core.bare = true in it).
var strippedEnv = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_COMMON_DIR",
	"GIT_PREFIX",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_NAMESPACE",
}

// Env returns environ (os.Environ when nil) without the variables that select a
// repository. Use it for any process that runs git on the caller's behalf.
func Env(environ []string) []string {
	if environ == nil {
		environ = runner.Environ()
	}
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if isStripped(name) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func isStripped(name string) bool {
	for _, s := range strippedEnv {
		if name == s {
			return true
		}
	}
	return false
}

// Command builds a git invocation whose environment carries no repository
// selection. A non-empty dir becomes `-C dir`, the explicit way to target a
// repository; an empty dir runs git in the process working directory (clone and
// version probes). Callers set Stdin, Stdout and Stderr as they would on an
// exec.Cmd.
func Command(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := runner.Command(ctx, "git", gitArgs(dir, args)...)
	cmd.Env = Env(nil)
	return cmd
}

// CommandNoContext is Command for callers that have no context to pass.
func CommandNoContext(dir string, args ...string) *exec.Cmd {
	cmd := runner.CommandNoContext("git", gitArgs(dir, args)...)
	cmd.Env = Env(nil)
	return cmd
}

func gitArgs(dir string, args []string) []string {
	if dir == "" {
		return args
	}
	return append([]string{"-C", dir}, args...)
}
