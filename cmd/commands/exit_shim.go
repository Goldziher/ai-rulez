package commands

import "os"

// TEMPORARY transitional helpers while handlers move to RunE; deleted at the end of WP-0.

func fmtError(err error) { renderError(os.Stderr, err) }

func fmtErrorFormat(format string, err error) {
	renderError(os.Stderr, err)
	if format == formatJSON {
		_ = writeErrorDocument(os.Stdout, err, exitCodeFor(err))
	}
}

func exitOnFormat(format string, err error) {
	if err != nil {
		fmtErrorFormat(format, err)
		os.Exit(1)
	}
}

func fatal(msg string, err error) {
	renderError(os.Stderr, failMsg(msg, err))
	os.Exit(1)
}
