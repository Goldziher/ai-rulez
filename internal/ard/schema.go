package ard

import (
	_ "embed"
	"io"
	"sync"

	"github.com/kaptinlin/jsonschema"
	"github.com/samber/oops"
)

// The entry schema is vendored, never fetched: see schema/NOTICE.txt.
const (
	// SchemaCommit is the ards-project/ard-spec commit the schema was copied from.
	SchemaCommit = "b76f235a8f461876ad4f1e77abd0eb0eb302b48d"
	// SchemaSHA256 is the SHA-256 of the vendored file; a test checks it.
	SchemaSHA256 = "011b86d55fd5d2883dffae3f0577d26f5efb56ca866eb079edbc78a628f95499"
	// schemaID is the vendored schema's $id.
	schemaID = "https://raw.githubusercontent.com/ards-project/ard-spec/main/spec/schemas/ard-entry.schema.json"
)

//go:embed schema/ard-entry.schema.json
var entrySchema []byte

// The vendored schema's root is ArdEntry; the manifest is its ArdManifest
// definition, reached through a wrapper that references it by $id.
const manifestSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","$ref":"` +
	schemaID + `#/$defs/ArdManifest"}`

// validators holds the compiled ArdEntry (the vendored schema's root) and
// ArdManifest schemas. Format assertion is on, so url must be a URI and
// updatedAt a date-time.
type validators struct {
	entry    *jsonschema.Schema
	manifest *jsonschema.Schema
}

var loadValidators = sync.OnceValues(func() (*validators, error) {
	c := jsonschema.NewCompiler().SetAssertFormat(true)
	// Never reach the network: every reference resolves to the vendored copy.
	offline := func(url string) (io.ReadCloser, error) {
		return nil, oops.Errorf("ard: schema reference %s is not vendored", url)
	}
	c.RegisterLoader("http", offline)
	c.RegisterLoader("https", offline)
	entry, err := c.Compile(entrySchema, schemaID)
	if err != nil {
		return nil, oops.Wrapf(err, "compiling the vendored ARD entry schema")
	}
	manifest, err := c.Compile([]byte(manifestSchema))
	if err != nil {
		return nil, oops.Wrapf(err, "compiling the ARD manifest schema")
	}
	return &validators{entry: entry, manifest: manifest}, nil
})
