// Package jsondoc writes the JSON documents of `--format json` with a leading
// "schema_version" so every machine-readable output of the CLI is versioned.
package jsondoc

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/samber/oops"
)

// Version is the schema_version of the documents written through this package.
// A document whose shape changes incompatibly bumps it and its schema file.
const Version = 1

const versionKey = "schema_version"

// Marshal renders doc as indented JSON that carries a top-level "schema_version"
// (placed first). A JSON object keeps its own schema_version when it has one; a
// JSON array is wrapped as {"schema_version": N, "items": [...]}. Anything else
// is an error: a scalar cannot carry a version.
func Marshal(doc any) ([]byte, error) {
	var enc bytes.Buffer
	encoder := json.NewEncoder(&enc)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(doc); err != nil {
		return nil, oops.Wrapf(err, "encode json document")
	}
	raw := bytes.TrimSpace(enc.Bytes())
	var buf bytes.Buffer
	switch {
	case bytes.HasPrefix(raw, []byte("{")):
		if hasVersion(raw) {
			buf.Write(raw)
			break
		}
		buf.WriteString(`{"` + versionKey + `":1`)
		if len(raw) > 2 {
			buf.WriteByte(',')
		}
		buf.Write(raw[1:])
	case bytes.HasPrefix(raw, []byte("[")):
		buf.WriteString(`{"` + versionKey + `":1,"items":`)
		buf.Write(raw)
		buf.WriteByte('}')
	default:
		return nil, oops.Errorf("a json document must be an object or an array, got %s", raw)
	}
	var out bytes.Buffer
	if err := json.Indent(&out, buf.Bytes(), "", "  "); err != nil {
		return nil, oops.Wrapf(err, "indent json document")
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

// Write writes the Marshal form of doc to w.
func Write(w io.Writer, doc any) error {
	data, err := Marshal(doc)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return oops.Wrapf(err, "write json document")
	}
	return nil
}

func hasVersion(raw []byte) bool {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return false
	}
	_, ok := keys[versionKey]
	return ok
}
