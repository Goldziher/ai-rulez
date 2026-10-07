package commands

import (
	"io"
	"os"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
)

// writeErrorDocument writes the `--format json` document of a command that
// failed before it could produce its report: schema_version, the error message and, when there is one, a hint. A consumer that parses
// stdout then always gets a document.
func writeErrorDocument(w io.Writer, err error) error {
	doc := map[string]string{"status": "error", "error": err.Error()}
	if oopsErr, ok := oops.AsOops(err); ok && oopsErr.Hint() != "" {
		doc["hint"] = oopsErr.Hint()
	}
	return jsondoc.Write(w, doc)
}

// fmtErrorFormat prints err for a human on stderr and, under --format json, the
// error document on stdout as well.
func fmtErrorFormat(format string, err error) {
	fmtError(err)
	if format == formatJSON {
		_ = writeErrorDocument(os.Stdout, err) //nolint:errcheck // the error is already reported on stderr
	}
}

// exitOnFormat is exitOn for a command with a --format flag.
func exitOnFormat(format string, err error) {
	if err != nil {
		fmtErrorFormat(format, err)
		os.Exit(1)
	}
}
