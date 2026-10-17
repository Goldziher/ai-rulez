package commands

import (
	"context"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/sessionstart"
)

// SessionStartCmd is the SessionStart hook that `[session_start]` adds to
// .claude/settings.json. The harness runs it, but it is a documented command a
// person can try, so it is listed in the help.
var SessionStartCmd = newSessionStartCmd(defaultSessionStartDeps)

func defaultSessionStartDeps() sessionstart.Deps {
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	return sessionstart.Deps{
		Getenv:  os.Getenv,
		Environ: os.Environ,
		Getwd:   os.Getwd,
		Runner:  runner.Exec{},
		Exe:     exe,
		TempDir: os.TempDir(),
		UID:     os.Getuid(),
		Now:     time.Now,
		LoadConfig: func(ctx context.Context, dir string) (*config.Config, error) {
			return loadProject(ctx, dir, config.WithoutRemote(), config.WithoutLocal())
		},
	}
}

func newSessionStartCmd(deps func() sessionstart.Deps) *cobra.Command {
	var ov sessionstart.Overrides
	var timeoutSeconds int
	cmd := &cobra.Command{
		Use:   "session-start",
		Short: "Regenerate outputs and brief the agent when a remote session starts (SessionStart hook)",
		Long: `Run by the SessionStart hook that [session_start] adds to .claude/settings.json.

In a remote agent session (CLAUDE_CODE_REMOTE=true) it runs ` + "`ai-rulez generate`" + ` for the project, so the
outputs of a fresh clone are present and current, then prints the hook's JSON document with a short
additionalContext that tells the agent the generated files are managed by ai-rulez and, when they are enabled, how to
use the ai-rulez MCP server and served skills. Elsewhere it does nothing, unless [session_start] when = "always".

The run uses --locked when ai-rulez.lock exists (mode auto), is bounded by a timeout that kills the whole generate
process tree, holds a lock so two hooks starting together do not race, and runs generate with a scrubbed environment.
It never fails a session: the exit code is always 0, and a problem is one line on stderr. stdout carries only the
JSON document. A project with no ai-rulez configuration is a silent no-op.`,
		Example: `  echo '{"source":"startup"}' | CLAUDE_CODE_REMOTE=true ai-rulez session-start
  ai-rulez session-start --when always --generate-mode plain`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ov.Timeout = time.Duration(timeoutSeconds) * time.Second
			sessionstart.Run(cmdContext(), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), deps(), ov)
			return nil
		},
	}
	cmd.Flags().StringVar(&ov.When, "when", "", `Override [session_start] when: "remote" or "always"`)
	cmd.Flags().StringVar(&ov.Mode, "generate-mode", "", `Override [session_start] mode: "auto", "locked", "frozen" or "plain"`)
	cmd.Flags().IntVar(&timeoutSeconds, "timeout", 0, "Override [session_start] timeout, in seconds")
	return cmd
}
