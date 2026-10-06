package publish

import (
	"context"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// uploadTimeout bounds one gh invocation (an upload can be large).
const uploadTimeout = 10 * time.Minute

// ReleaseCreateArgv is the fixed argv that creates the release. The tag must
// already exist on the remote (--verify-tag): publishing never creates tags.
func ReleaseCreateArgv(name, version, tag, repo string, upload []string) []string {
	argv := []string{
		"gh", "release", "create", tag, "--repo", repo, "--title", name + " " + version,
		"--notes-file", NotesFile, "--verify-tag",
	}
	return append(argv, upload...)
}

// ReleaseUploadArgv replaces the assets of an existing release (--force).
func ReleaseUploadArgv(tag, repo string, upload []string) []string {
	argv := []string{"gh", "release", "upload", tag, "--repo", repo, "--clobber"}
	return append(argv, upload...)
}

// ExecuteOptions configure Execute.
type ExecuteOptions struct {
	// Dir is the written dist directory, the working directory of gh.
	Dir string
	// Env is the complete environment of gh, built by the caller (runner.ScrubEnv).
	Env []string
	// Force replaces the assets of an existing release instead of refusing.
	Force bool
}

// Execute runs the upload through the gh CLI and returns its stdout (the
// release URL). ai-rulez never sees a credential: gh authenticates itself from
// the environment the caller passed.
func Execute(ctx context.Context, r runner.Runner, plan Plan, opts ExecuteOptions) (string, error) {
	if plan.Target != TargetGitHubRelease || len(plan.Commands) != 1 {
		return "", newError(CodeTarget, ExitFailed, "", "the plan has no github-release command to run")
	}
	r = runner.Or(r)
	run := func(argv []string) runner.Result {
		return r.Run(ctx, runner.Spec{Argv: argv, Dir: opts.Dir, Env: opts.Env, Timeout: uploadTimeout})
	}
	view := run([]string{"gh", "release", "view", plan.Tag, "--repo", plan.Repo})
	exists := false
	switch view.Status {
	case runner.StatusOK:
		exists = true
	case runner.StatusUnavailable:
		return "", newError(CodeTarget, ExitFailed, "install the GitHub CLI (https://cli.github.com) and run `gh auth login`", "gh was not found on PATH")
	case runner.StatusExit:
		if !strings.Contains(strings.ToLower(string(view.Stderr)), "not found") {
			return "", ghFailure("gh release view", view)
		}
	default:
		return "", ghFailure("gh release view", view)
	}
	argv := plan.Commands[0].Argv
	if exists {
		if !opts.Force {
			return "", newError(CodeTarget, ExitFailed, "pass --force to replace the assets of the existing release",
				"release %s already exists in %s; releases are immutable by default", plan.Tag, plan.Repo)
		}
		argv = ReleaseUploadArgv(plan.Tag, plan.Repo, plan.Upload)
	}
	res := run(argv)
	if res.Status != runner.StatusOK {
		return "", ghFailure(strings.Join(argv[:3], " "), res)
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}

func ghFailure(what string, res runner.Result) error {
	if res.Status == runner.StatusUnavailable {
		return newError(CodeTarget, ExitFailed, "install the GitHub CLI (https://cli.github.com)", "gh was not found on PATH")
	}
	detail := strings.TrimSpace(string(res.Stderr))
	if detail == "" && res.Err != nil {
		detail = res.Err.Error()
	}
	return newError(CodeTarget, ExitFailed, "", "%s failed (%s): %s", what, res.Status, detail)
}
