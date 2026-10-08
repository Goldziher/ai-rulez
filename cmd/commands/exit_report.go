package commands

import "os"

// renderStderr reports a failure on stderr the way the root renderer does. The
// run helpers that return an exit code (not an error) call it for the failure
// they are about to turn into that code; a RunE that can return the error
// itself returns fail(err) instead.
func renderStderr(err error) { renderError(os.Stderr, err) }
