package commands

import (
	"os"

	"github.com/spf13/cobra"
)

// activeCommand is the command whose RunE is running, so a helper that reports
// its own failure (and returns an exit code) knows which --format was asked for.
var (
	activeCommand        *cobra.Command
	errorDocumentWritten bool
)

// trackedRunE records which commands already report their active state.
var trackedRunE = map[*cobra.Command]bool{}

// trackActiveCommand wraps every RunE so activeCommand names the running
// command. Wrapping twice (a test calls execute repeatedly) is a no-op.
func trackActiveCommand(root *cobra.Command) {
	for _, c := range root.Commands() {
		trackActiveCommand(c)
	}
	if root.RunE == nil || trackedRunE[root] {
		return
	}
	trackedRunE[root] = true
	original := root.RunE
	root.RunE = func(cmd *cobra.Command, args []string) error {
		activeCommand, errorDocumentWritten = cmd, false
		return original(cmd, args)
	}
}

// renderStderr reports a failure the way the root renderer does: the text on
// stderr and, under --format json, the error document on stdout (once per run).
// The run helpers that return an exit code (not an error) call it for the failure
// they are about to turn into that code; a RunE that can return the error
// itself returns fail(err) instead.
func renderStderr(err error) {
	renderError(os.Stderr, err)
	if commandFormat(activeCommand) == formatJSON && !errorDocumentWritten {
		errorDocumentWritten = true
		_ = writeErrorDocument(os.Stdout, err, exitCodeFor(err)) //nolint:errcheck // the error is already reported on stderr
	}
}
