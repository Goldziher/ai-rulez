package conformance

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ard"
)

const (
	ardEntryID    = "https://raw.githubusercontent.com/ards-project/ard-spec/main/spec/schemas/ard-entry.schema.json"
	ardManifestID = "urn:conformance:ard-manifest"
)

func TestARDManifestGoldenConformsToTheEntrySchema(t *testing.T) {
	// Arrange
	golden := repoFile(t, "internal/ard/testdata/golden/ard.json")
	docs := map[string][]byte{
		ardEntryID:    schemaFile(t, "ard/ard-entry.schema.json"),
		ardManifestID: []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$ref":"` + ardEntryID + `#/$defs/ArdManifest"}`),
	}
	manifest := compileDocs(t, ardManifestID, docs)
	entry := compileDocs(t, ardEntryID, docs)

	// Assert: the manifest, then every entry on its own
	requireValid(t, manifest, golden)
	var doc struct {
		Entries []json.RawMessage `json:"entries"`
	}
	require.NoError(t, json.Unmarshal(golden, &doc))
	require.NotEmpty(t, doc.Entries)
	for _, e := range doc.Entries {
		requireValid(t, entry, e)
	}
	requireInvalid(t, manifest, `{"entries":[{"identifier":"not-a-urn"}]}`)
	requireInvalid(t, entry, `{"identifier":"urn:air:example.com:ns:x","displayName":"x","type":"application/ai-skill+md"}`)
}

func TestARDValidatorAcceptsTheGoldenAndNamesItsSpecVersion(t *testing.T) {
	findings, err := ard.Validate(repoFile(t, "internal/ard/testdata/golden/ard.json"))
	require.NoError(t, err)
	assert.Empty(t, findings)
	assert.Equal(t, "0.91", ard.SpecVersion)
}
