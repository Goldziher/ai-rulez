package commands

import (
	"encoding/json"
	"io"

	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
)

// writeErrorDocument writes the `--format json` document of a command that
// failed before it could produce its report: schema_version, the error message and, when there is one, a hint. A consumer that parses
// stdout then always gets a document.
func writeErrorDocument(w io.Writer, err error, code int) error {
	doc := errorDocument{Status: errorDocumentStatus, Error: errorText(err), ExitCode: code}
	doc.Hint = errorHintOf(err)
	return jsondoc.Write(w, doc)
}

// errorDocumentStatus is the status member of an error document.
const errorDocumentStatus = "error"

// errorDocument is the `--format json` document of a failed command.
type errorDocument struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Hint   string `json:"hint,omitempty"`
	// ExitCode is the process exit code of the failure.
	ExitCode int `json:"exit_code"`
}

// writeRawJSON encodes doc to w as two-space-indented JSON exactly as the
// report documents carry it (their own schema_version member, HTML escaping on).
func writeRawJSON(w io.Writer, doc any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
