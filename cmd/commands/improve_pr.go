package commands

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/improve"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/sandbox"
)

var improvePRFlags struct {
	base     string
	remote   string
	draft    bool
	noPush   bool
	runEvals bool
	evalArgs []string
	envPass  []string
}

// improvePRChildEnvPass are the non-secret variables the ai-rulez commands run in the worktree keep besides the
// base set (PATH, HOME, the temp directories and the locale): where the user and cache configuration lives.
var improvePRChildEnvPass = []string{"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "AI_RULEZ_HOME"}

// improvePRChildEnv is the environment of `ai-rulez generate`, `lock` and `eval run` in the worktree. The
// branch content is repository-controlled (a hook, an MCP command, an eval runner command), so these children
// get a scrubbed environment, not the host's: credentials reach them only by name with --env-pass (an eval
// runner needs its model key there).
func improvePRChildEnv(host, pass []string) []string {
	return runner.ScrubEnv(gitutil.Env(host), append(append([]string(nil), improvePRChildEnvPass...), pass...), nil)
}

var improvePRCmd = &cobra.Command{
	Use:   "pr <run-id>",
	Short: "(experimental) Open a pull request for an accepted improve run from an isolated worktree",
	Long: `Turn an accepted run into a branch and a pull request without touching your checkout.

It creates a linked git worktree from --base (default: the branch you are on), applies the candidate there
after checking that the skill at the base is the one the run measured (AR9J1) and that the candidate still
passes the diff policy, runs "ai-rulez generate" (staging the outputs the project commits) and, when the project has a
lock, "ai-rulez lock", and commits on the new branch ai-rulez/improve/<skill>-<digest8>. Those commands run with a
scrubbed environment: only the base variables and the names in --env-pass reach them. With --run-evals it also runs "ai-rulez eval run <skill> --changed-only"
in the worktree (it calls the eval runner and spends money; --eval-arg passes extra arguments such as
--max-cost) so the lock check and AR997 pass; without it the command tells you to run it on the branch.

When the remote exists, gh is on PATH, the base is a branch the remote has and holds no unpushed commits, and you
confirm (or pass --yes), it pushes the branch with git and opens the pull request with gh using fixed arguments
(--repo names the repository the remote URL points at); otherwise it prints the two commands. improve pr itself makes no network
call, but the generate and lock it runs in the worktree fetch remote includes and sources as they do anywhere. The pull request body names what changed, the held-out numbers with their interval, the guards,
the cost and egress, and a reviewer checklist, and says the change is NOT approved: nothing here sets approval.
The worktree is removed afterwards; the branch stays. Commits skip git hooks. Refusals carry AR9J8.

With --isolation auto|require (or [improve] isolation) the commands run in the worktree run under the process
sandbox: writable only below the worktree, the user cache, ai-rulez's state directory and the temp directory, with
the network on (generate and lock fetch remote includes; eval run calls a model). require refuses (AR9J7) when no
backend works. A harness that keeps its state elsewhere needs --isolation none for --run-evals.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := commandContext(cmd)
		reportWriter{cmd.ErrOrStderr()}.printf("%s\n", improveExperimental)
		if err := checkFormatFlag(improveFlags.format); err != nil {
			return err
		}
		cfg, err := loadConfigForCommand(ctx, nil)
		if err != nil {
			return err
		}
		configDirAbs, err := filepath.Abs(cfg.ConfigDir)
		if err != nil {
			return oops.Wrapf(err, "resolve config directory")
		}
		repo, err := filepath.Abs(cfg.BaseDir)
		if err != nil {
			return oops.Wrapf(err, "resolve project directory")
		}
		asJSON := improveFlags.format == formatJSON
		out := cmd.OutOrStdout()
		if asJSON {
			out = cmd.ErrOrStderr() // stdout carries the result document only
		}
		isolation, err := resolvePRIsolationMode(cmd, cfg)
		if err != nil {
			return err
		}
		opts := &improve.PROptions{
			ConfigDir: configDirAbs, RepoDir: repo, RunID: args[0], Base: improvePRFlags.base, Remote: improvePRFlags.remote,
			Draft: improvePRFlags.draft, NoPush: improvePRFlags.noPush, Yes: improveFlags.yes, Confirm: confirmProceed, Out: out,
			Git: gitutil.Git{}, Exec: runner.Exec{}, RunEvals: improvePRFlags.runEvals, EvalArgs: improvePRFlags.evalArgs,
			GHEnv: runner.ScrubEnv(os.Environ(), ghEnvPass, []string{"GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1"}),
			Env:   improvePRChildEnv(os.Environ(), improvePRFlags.envPass), AllowScripts: improveFlags.allowScripts, AllowFrontmatter: improveFlags.allowFrontmatter,
			Isolation: isolation,
		}
		if self, serr := improveSelf(); serr == nil {
			opts.Self = self
		}
		res, err := improve.PR(ctx, opts)
		if err != nil {
			return oops.Wrap(err)
		}
		if asJSON {
			return writeImproveJSON(cmd.OutOrStdout(), res)
		}
		if len(res.Refreshed) > 0 {
			reportWriter{out}.printf("Ran in the worktree: %s\n", strings.Join(res.Refreshed, "; "))
		}
		return nil
	},
}

// resolvePRIsolationMode is the confinement of the commands run in the worktree: --isolation, else [improve]
// isolation, else none.
func resolvePRIsolationMode(cmd *cobra.Command, cfg *config.Config) (sandbox.Mode, error) {
	mode := improveFlags.isolation
	if !cmd.Flags().Changed("isolation") {
		res, err := cfg.ResolveImprove(false, nil)
		if err != nil {
			return "", oops.Wrap(err)
		}
		mode = res.Effective.Isolation
	}
	if mode == "" {
		return sandbox.ModeNone, nil
	}
	parsed, err := sandbox.ParseMode(mode)
	return parsed, oops.Wrap(err)
}

func init() {
	f := improvePRCmd.Flags()
	f.StringVar(&improveFlags.isolation, "isolation", "", "Confine the ai-rulez commands run in the worktree (generate, lock, eval run): none (default), auto (when a sandbox backend works) or require (refuse without one)")
	f.StringVar(&improvePRFlags.base, "base", "", "Branch or commit the worktree starts from and the pull request targets (default: the current branch)")
	f.StringVar(&improvePRFlags.remote, "remote", improve.DefaultRemote, "Remote to push the branch to")
	f.BoolVar(&improvePRFlags.draft, "draft", false, "Open the pull request as a draft")
	f.BoolVar(&improvePRFlags.noPush, "no-push", false, "Commit on the branch only: no push, no pull request")
	f.BoolVar(&improvePRFlags.runEvals, "run-evals", false, "Also run eval run <skill> with --changed-only in the worktree (calls the eval runner and costs money)")
	f.StringArrayVar(&improvePRFlags.evalArgs, "eval-arg", nil, "Extra argument for eval run with --run-evals, for example --eval-arg=--max-cost=2; repeatable")
	f.StringSliceVar(&improvePRFlags.envPass, "env-pass", nil, "Environment variable names forwarded to the ai-rulez commands run in the worktree (all others are scrubbed); eval run needs its model credentials here")
	f.BoolVarP(&improveFlags.yes, "yes", "y", false, "Push and open the pull request without the confirmation prompt")
	f.BoolVar(&improveFlags.allowScripts, "allow-scripts", false, "Allow the candidate to change scripts/ and assets/ and reference scripts")
	f.BoolVar(&improveFlags.allowFrontmatter, "allow-frontmatter", false, "Allow the candidate to change allowed-tools, model and disable-model-invocation")
	addFormatFlag(f, &improveFlags.format, formatText, formatText, formatText, formatJSON)
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	ImproveCmd.AddCommand(improvePRCmd)
}
