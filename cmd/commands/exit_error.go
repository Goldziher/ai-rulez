package commands

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// The exit-code contract. Every command follows it and main is the only place
// that turns it into a process exit (see Main).
const (
	// exitOK: the command ran and found nothing to report.
	exitOK = 0
	// exitFailure: the command could not run (configuration, usage, I/O, a tool
	// or network error, a refused confirmation).
	exitFailure = 1
	// exitFindings: the command ran and found something (findings at or above
	// --fail-on, drift, a budget or gate failed, a policy the configuration
	// loosens). It is the code `generate --check`, `validate` and `lock --check`
	// share.
	exitFindings = 2
	// exitPartial: `lock` only, the lock was written but served skills stayed
	// unpinned because the security scan refuses them.
	exitPartial = 3
)

// ExitError is how a command ends with a specific exit code. A RunE returns it
// (or an error that exitCodeFor maps to a code) instead of calling os.Exit, so a
// handler is testable in-process and its deferred cleanup runs.
//
// Silent marks an error the command has already reported (a findings report, a
// summary line): the root renderer then prints nothing more and only sets the
// code. Without Silent the root renders Err once, in the format the command
// was asked for.
type ExitError struct {
	Code   int
	Err    error
	Silent bool
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit status %d", e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }

// exitStatus ends a command whose result is already printed with code; 0 is
// success, so it returns nil.
func exitStatus(code int) error {
	if code == exitOK {
		return nil
	}
	return &ExitError{Code: code, Silent: true}
}

// failWithCode ends a command with code and err, which the root renders once.
func failWithCode(code int, err error) error {
	return &ExitError{Code: code, Err: err}
}

// fail ends a command with err and the exit code the contract gives it.
func fail(err error) error {
	if err == nil {
		return nil
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return err
	}
	return &ExitError{Code: exitCodeFor(err), Err: err}
}

// failMsg is fail for a failure that has a headline of its own: "msg: err".
func failMsg(msg string, err error) error {
	return fail(oops.Wrapf(err, "%s", msg))
}

// exitCodeFor is the exit code of a failed command: an ExitError's own code;
// 2 for a role that names something that does not exist (AR971), a
// configuration that loosens the organization policy or drift from the lock or
// a pinned tag, the code `validate` gives findings; else 1.
func exitCodeFor(err error) int {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	switch {
	case errors.Is(err, config.ErrRoleReference), errors.Is(err, config.ErrPolicyLoosens),
		errors.Is(err, errLockedSourceDrift), errors.Is(err, errLockedSignature),
		errors.Is(err, errLockedApproval), errors.Is(err, errMovedTag):
		return exitFindings
	}
	return exitFailure
}

// Main runs the CLI and returns the process exit code, after rendering a
// failure once. It is the only function that cmd/ai-rulez/main.go needs; the
// os.Exit lives there.
func Main() int {
	cmd, err := execute()
	if err == nil {
		return exitOK
	}
	return ReportError(os.Stdout, os.Stderr, commandFormat(cmd), err)
}

// ReportError renders err the one way every command does and returns its exit
// code. The text goes to stderr ("Error: ...", the validation errors and a
// "Hint: ..." line); under --format json the error document goes to stdout as
// well, so a consumer that parses stdout always gets a document.
func ReportError(stdout, stderr io.Writer, format string, err error) int {
	code := exitCodeFor(err)
	var exitErr *ExitError
	if errors.As(err, &exitErr) && (exitErr.Silent || exitErr.Err == nil) {
		return code
	}
	renderError(stderr, err)
	if format == formatJSON {
		_ = writeErrorDocument(stdout, err, code) //nolint:errcheck // the error is already reported on stderr
	}
	return code
}

// commandFormat is the --format a command was asked for, "" when it has none
// (or when the failure came before the command was found).
func commandFormat(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	if f := cmd.Flags().Lookup("format"); f != nil {
		return f.Value.String()
	}
	return ""
}

// renderError is the one text rendering of a failure.
func renderError(w io.Writer, err error) {
	fmt.Fprintf(w, "Error: %s\n", errorText(err))
	if details := errorDetails(err); len(details) > 0 {
		fmt.Fprintf(w, "\nValidation errors:\n")
		for _, d := range details {
			fmt.Fprintf(w, "  - %s\n", d)
		}
	}
	if hint := errorHintOf(err); hint != "" {
		fmt.Fprintf(w, "\nHint: %s\n", hint)
	}
}

// errorText is the message of err without the ExitError wrapper.
func errorText(err error) string {
	var exitErr *ExitError
	if errors.As(err, &exitErr) && exitErr.Err != nil {
		return exitErr.Err.Error()
	}
	return err.Error()
}

func errorDetails(err error) []string {
	if oopsErr, ok := oops.AsOops(err); ok {
		if details, ok := oopsErr.Context()["errors"].([]string); ok {
			return details
		}
	}
	return nil
}

func errorHintOf(err error) string {
	if oopsErr, ok := oops.AsOops(err); ok {
		return errorHint(oopsErr)
	}
	return ""
}
