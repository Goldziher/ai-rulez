package commands

import (
	"fmt"
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
Code, Codex, Gemini CLI, Cursor and Factory block a tool call. Every other case
exits 0: a path that is not a wholly owned output (settings files ai-rulez only
merges into are allowed), a path outside the project, a read-only tool and a
payload that cannot be parsed. The guard fails open.

Add the hook with [guard] generated = true in .ai-rulez/config.toml.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, _ []string) {
		cwd, _ := os.Getwd()
		if code := runGuard(cmd.InOrStdin(), cmd.ErrOrStderr(), cwd); code != 0 {
			os.Exit(code)
		}
	},
}

// runGuard returns the hook exit code: guard.ExitBlock for a blocked call, 0 otherwise.
func runGuard(stdin io.Reader, stderr io.Writer, cwd string) int {
	decision := guard.Check(stdin, cwd)
	if !decision.Block {
		return 0
	}
	_, _ = fmt.Fprint(stderr, decision.Message())
	return guard.ExitBlock
}
