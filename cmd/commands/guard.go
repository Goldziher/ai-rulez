package commands

import (
	"io"
	"os"

	"github.com/Goldziher/ai-rulez/v5/internal/guard"
	"github.com/spf13/cobra"
)

// GuardCmd is the PreToolUse hook that `[guard] generated = true` adds to each
// harness. It is hidden: people never run it, the harness does.
var GuardCmd = &cobra.Command{
	Use:    "guard",
	Short:  "Block agent edits to generated files (PreToolUse hook)",
	Hidden: true,
	Long: `Read a harness PreToolUse hook payload on stdin and block the tool call when it
edits a file ai-rulez generated.

A blocked call exits with code 2 and the reason on stderr, which is how Claude
Code, Codex, Gemini CLI, Cursor, Factory and Copilot block a tool call. Every other case
exits 0: a path that is not a wholly owned output (settings files ai-rulez only
merges into are allowed), a path outside the project, a read-only tool and a
payload that cannot be parsed. The guard fails open on its own errors; a payload
over 8 MiB or a call naming over 1000 files is blocked.

Add the hook with [guard] generated = true in .ai-rulez/config.toml.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "" // the guard fails open on its own errors
		}
		return exitStatus(runGuard(cmd.InOrStdin(), cmd.ErrOrStderr(), cwd))
	},
}

// runGuard returns the hook exit code: guard.ExitBlock for a blocked call, 0 otherwise.
func runGuard(stdin io.Reader, stderr io.Writer, cwd string) int {
	decision := guard.Check(stdin, cwd)
	if !decision.Block {
		return 0
	}
	reportWriter{stderr}.printf("%s", decision.Message())
	return guard.ExitBlock
}
