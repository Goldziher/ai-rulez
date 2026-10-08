package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/render"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// newContentOperator opens the CRUD operator for the current directory. With
// local set, content operations work on the machine-local tree
// (.ai-rulez/local/), which mirrors the shared layout.
func newContentOperator(local bool) (*crud.OperatorImpl, error) {
	op, err := crud.NewOperator(".")
	if err != nil {
		return nil, err
	}
	if local {
		op = op.Local()
	}
	return op, nil
}

// ErrNeedsYes marks a destructive operation that was neither confirmed with
// --yes nor confirmable because there is no terminal to ask.
var ErrNeedsYes = errors.New("needs --yes")

// stdinInteractive reports whether standard input is a terminal; tests replace it.
var stdinInteractive = func() bool {
	stat, err := os.Stdin.Stat()
	return err == nil && stat.Mode()&os.ModeCharDevice != 0
}

// confirmInput is where an interactive confirmation reads its answer from.
var confirmInput io.Reader = os.Stdin

// confirm is the one confirmation of every destructive command. yes (--yes)
// confirms without asking. Otherwise it asks on stderr, so piped stdout stays
// clean, and returns nil only on "y" or "yes". A declined prompt and a
// non-interactive shell both return a failure (exit 1) that names declined, the
// text of what was not done; the latter wraps ErrNeedsYes.
func confirm(yes bool, prompt, declined string) error {
	if yes {
		return nil
	}
	hint := "pass --yes to skip the confirmation prompt (required in non-interactive shells)"
	if !stdinInteractive() {
		return oops.Hint(hint).Wrapf(ErrNeedsYes, "%s: not confirmed, nothing was changed", declined)
	}
	if !readYesNo(prompt) {
		return oops.Hint(hint).Errorf("%s: not confirmed, nothing was changed", declined)
	}
	return nil
}

// confirmRemovalUnlessYes asks for confirmation before a removal unless yes is
// set. declined names what was not done.
func confirmRemovalUnlessYes(yes bool, resourceType, resourceName, declined string) error {
	return confirm(yes, removalPrompt(resourceType, resourceName), declined)
}

func removalPrompt(resourceType, resourceName string) string {
	if resourceType == "" {
		return fmt.Sprintf("Are you sure you want to remove %s? (y/N): ", resourceName)
	}
	return fmt.Sprintf("Are you sure you want to remove %s '%s'? (y/N): ", resourceType, resourceName)
}

// addYesFlag registers the shared --yes/-y flag; usage is the command's own
// help text.
func addYesFlag(fs *pflag.FlagSet, dst *bool, usage string) {
	fs.BoolVarP(dst, "yes", "y", false, usage)
}

// askYesNo prints prompt on stderr and reads a yes/no answer from an
// interactive terminal; a pipe, CI or any other non-interactive input answers no.
func askYesNo(prompt string) bool {
	if !stdinInteractive() {
		return false
	}
	return readYesNo(prompt)
}

func readYesNo(prompt string) bool {
	fmt.Fprint(os.Stderr, prompt)

	var response string
	if _, err := fmt.Fscanln(confirmInput, &response); err != nil && err.Error() != "unexpected newline" {
		return false
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == answerYes
}

// workingDir is the process working directory, "" when it cannot be read. The
// CLI resolves it once here and passes it to library code that shows paths
// relative to it, so no library package reads the working directory itself.
func workingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

// outFor is the result/diagnostic writer pair of a running command: results go
// to its stdout, diagnostics to its stderr, and -q hides only the information
// lines of the latter.
func outFor(cmd *cobra.Command) render.Out {
	return render.New(cmd.OutOrStdout(), cmd.ErrOrStderr(), viper.GetBool("quiet"))
}

// writeListJSON writes items, the result of a list command, as a versioned JSON
// document to w.
func writeListJSON(w io.Writer, items []map[string]interface{}) error {
	data, err := jsondoc.Marshal(items)
	if err != nil {
		return failMsg("Failed to marshal JSON", err)
	}
	if _, err := w.Write(data); err != nil {
		return fail(err)
	}
	return nil
}

// writef writes formatted text to w. Output to a command's result stream has
// nowhere to report a failed write, so the error is dropped.
func writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...) //nolint:errcheck // see above
}

// writeln writes a line to w; see writef.
func writeln(w io.Writer, args ...any) {
	_, _ = fmt.Fprintln(w, args...) //nolint:errcheck // see above
}
