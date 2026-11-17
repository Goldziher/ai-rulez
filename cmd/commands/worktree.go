package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/improve"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/worktreeprovision"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// worktreeGenerateTimeout bounds the generate run in the destination: generate
// may fetch remote includes, but it never calls a model.
const worktreeGenerateTimeout = 30 * time.Minute

// worktreeFlags are the flags both worktree commands share; the constructor
// holds the one instance.
type worktreeFlags struct {
	format    string
	branch    string
	startRef  string
	copyLocal bool
	copyExtra []string
	dryRun    bool
}

// newWorktreeCmd builds the worktree command tree with its flags bound to one
// flags instance the RunE closures capture.
func newWorktreeCmd() *cobra.Command {
	flags := &worktreeFlags{}

	var create, setup *cobra.Command

	worktree := &cobra.Command{
		Use:   "worktree",
		Short: "Provision linked worktrees with this checkout's AI-Rulez configuration",
		Long: `Carry a checkout's AI-Rulez setup into a linked git worktree.

A fresh linked worktree inherits the tracked configuration from its branch, but the machine-local
overlay (.ai-rulez/config.local.toml), the local content tree (.ai-rulez/local/) and the
hand-authored local instruction and settings files (CLAUDE.local.md, AGENTS.local.md,
AGENTS.override.md, GEMINI.local.md, .claude/settings.local.json) stay behind. The worktree
commands scaffold the destination's outputs from the destination's own committed configuration
and carry the local inputs only when you ask.

Run the commands from the source checkout, the one that holds the configuration. The destination
must be a linked worktree of the same repository (git or your host can create it). Generated
manifests, cached artifacts and rendered credentials are never copied, and no file content is
printed. Copied files get owner-only permissions and are never followed through a symlink.

A failure reports a retryable state and leaves the worktree and its branch intact.`,
	}

	create = &cobra.Command{
		Use:   "create <destination>",
		Short: "Create a linked worktree on a new branch and provision it",
		Long: `Create a linked git worktree at <destination> on a new branch and provision it.

The branch starts from --start-ref (default: the source checkout's HEAD). The destination must
not exist and the branch must be new. Run it from the source checkout; see worktree --help for
what provisioning carries.`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorktreeProvision(cmd, flags, args[0], func(ctx context.Context, opts *worktreeprovision.Options) (*worktreeprovision.Result, error) {
				return worktreeprovision.Create(ctx, opts, flags.branch, flags.startRef)
			})
		},
	}
	create.Flags().StringVar(&flags.branch, "branch", "", "Branch the linked worktree checks out (must be new)")
	create.Flags().StringVar(&flags.startRef, "start-ref", "", "Branch or commit the worktree starts from (default: the source checkout's HEAD)")
	_ = create.MarkFlagRequired("branch") //nolint:errcheck // the flag exists

	setup = &cobra.Command{
		Use:   "setup <destination>",
		Short: "Provision an existing linked worktree",
		Long: `Provision a linked worktree that git or your host already created.

The destination must be a registered linked worktree of the same repository. Run it from the
source checkout; see worktree --help for what provisioning carries.`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorktreeProvision(cmd, flags, args[0], worktreeprovision.Setup)
		},
	}

	for _, cmd := range []*cobra.Command{create, setup} {
		f := cmd.Flags()
		f.BoolVar(&flags.copyLocal, "copy-local", false, "Carry the machine-local inputs: config.local.toml, .ai-rulez/local/ and the hand-authored CLAUDE.local.md, AGENTS.local.md, AGENTS.override.md, GEMINI.local.md and .claude/settings.local.json that exist in the source")
		f.StringArrayVar(&flags.copyExtra, "copy-extra", nil, "An extra source-relative file or directory to carry, on top of the allowlist; it must be ignored and untracked in the destination; repeatable")
		f.BoolVarP(&flags.dryRun, "dry-run", "n", false, "Plan the run and write nothing")
		addFormatFlag(f, &flags.format, "", formatText, formatText, formatJSON)
	}
	worktree.AddCommand(create, setup)
	return worktree
}

// runWorktreeProvision wires the flags, the generate step and the output
// format around one provisioning call.
func runWorktreeProvision(cmd *cobra.Command, flags *worktreeFlags, destination string, run func(ctx context.Context, opts *worktreeprovision.Options) (*worktreeprovision.Result, error)) error {
	ctx := commandContext(cmd)
	if err := checkFormatFlag(flags.format); err != nil {
		return err
	}
	asJSON := flags.format == formatJSON
	out := cmd.OutOrStdout()
	if asJSON {
		out = cmd.ErrOrStderr() // stdout carries the result document only
	}
	opts := &worktreeprovision.Options{
		Source:      ".",
		Destination: destination,
		CopyLocal:   flags.copyLocal,
		CopyExtra:   flags.copyExtra,
		DryRun:      flags.dryRun,
		Out:         out,
		Generate:    worktreeGenerateStep,
	}
	res, err := run(ctx, opts)
	if err != nil {
		return oops.Wrap(err)
	}
	if asJSON {
		return jsondoc.Write(cmd.OutOrStdout(), res) //nolint:wrapcheck // the write error is the command error
	}
	return nil
}

// worktreeGenerateStep regenerates the destination's outputs by running this
// binary's generate command there, so the run sees the same configuration
// resolution, warnings and overwrite guard as a hand-run generate.
func worktreeGenerateStep(ctx context.Context, projectDir string, dryRun bool) ([]string, error) {
	self, err := improveSelf()
	if err != nil {
		return nil, err
	}
	argv := append(append([]string(nil), self...), "generate", "--yes")
	if dryRun {
		argv = append(argv, "--dry-run")
	}
	r := runner.Exec{}.Run(ctx, runner.Spec{Argv: argv, Dir: projectDir, Env: os.Environ(), Timeout: worktreeGenerateTimeout})
	if r.Status != runner.StatusOK {
		return nil, fmt.Errorf("`ai-rulez generate` failed in the destination (%s, exit %d): %s", r.Status, r.ExitCode, improve.Sanitize(string(r.Stderr)+string(r.Stdout), 600))
	}
	return strings.Split(strings.TrimRight(string(r.Stdout), "\n"), "\n"), nil
}
