package sbom

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/samber/oops"
)

// Write encodes bom as indented JSON with a trailing newline. Output is
// byte-stable: struct field order is fixed and every slice is sorted by Build.
func Write(w io.Writer, bom *BOM) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(bom); err != nil {
		return oops.Wrapf(err, "encode sbom")
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		return oops.Wrapf(err, "write sbom")
	}
	return nil
}
