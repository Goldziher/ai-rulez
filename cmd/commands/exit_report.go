package commands

import (
	"os"

	"github.com/spf13/cobra"
)

// The state of the running command lives on the root command's annotations, not
// in package variables: the --format it was asked for and whether an error
// document was already written.
const (
	activeFormatKey  = "ai-rulez-active-format"
	errorDocWritten  = "ai-rulez-error-document-written"
	runTrackedMarker = "ai-rulez-run-tracked"
)

// trackActiveCommand wraps every RunE so the root knows which --format the
// running command was asked for. Wrapping twice (a test calls execute
// repeatedly) is a no-op.
func trackActiveCommand(root *cobra.Command) {
	for _, c := range root.Commands() {
		trackActiveCommand(c)
	}
	if root.RunE == nil || root.Annotations[runTrackedMarker] != "" {
		return
	}
	if root.Annotations == nil {
		root.Annotations = map[string]string{}
	}
	root.Annotations[runTrackedMarker] = "1"
	original := root.RunE
	root.RunE = func(cmd *cobra.Command, args []string) error {
		RootCmd.Annotations[activeFormatKey] = commandFormat(cmd)
		delete(RootCmd.Annotations, errorDocWritten)
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
	if RootCmd.Annotations[activeFormatKey] == formatJSON && RootCmd.Annotations[errorDocWritten] == "" {
		RootCmd.Annotations[errorDocWritten] = "1"
		_ = writeErrorDocument(os.Stdout, err, exitCodeFor(err)) //nolint:errcheck // the error is already reported on stderr
	}
}
