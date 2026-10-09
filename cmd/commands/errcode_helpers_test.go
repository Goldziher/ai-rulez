package commands

import "os"

// codeOf is the process exit code a returned error ends a command with; 0 for nil.
func codeOf(err error) int {
	if err == nil {
		return exitOK
	}
	return exitCodeFor(err)
}

// reported renders err the way the root does (text on stderr, the error
// document on stdout under --format json) and returns its exit code, so a test
// that captures the streams of a run helper sees what a user would.
func reported(err error) int {
	if err == nil {
		return exitOK
	}
	return ReportError(os.Stdout, os.Stderr, RootCmd.Annotations[activeFormatKey], err)
}
