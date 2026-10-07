package ard

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVendoredSchemaMatchesThePin(t *testing.T) {
	// Arrange
	data, err := os.ReadFile(filepath.Join("schema", "ard-entry.schema.json"))
	require.NoError(t, err)

	// Act
	sum := sha256.Sum256(data)

	// Assert
	assert.Equal(t, SchemaSHA256, hex.EncodeToString(sum[:]), "the vendored schema changed: update SchemaCommit and SchemaSHA256")
	assert.Equal(t, data, entrySchema)
}

const (
	okEntry   = `{"identifier":"urn:air:example.com:skills:deploy","displayName":"deploy","type":"application/ai-skill+md","url":"https://example.com/deploy","representativeQueries":["ship it","deploy the service"]}`
	okEntry2  = `{"identifier":"urn:air:example.com:skills:review","displayName":"review","type":"application/ai-skill+md","data":{"a":1},"representativeQueries":["review this","check my diff"]}`
	noQueries = `{"identifier":"urn:air:example.com:skills:x","displayName":"x","type":"application/ai-skill+md","url":"https://example.com/x"}`
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		doc       string
		wantErr   bool
		wantRules []string
	}{
		{name: "valid manifest", doc: `{"entries":[` + okEntry + `,` + okEntry2 + `]}`},
		{name: "empty entries", doc: `{"entries":[]}`},
		{name: "missing entries", doc: `{}`, wantRules: []string{RuleSchema}},
		{name: "not json", doc: `{`, wantErr: true},
		{
			name:      "url and data",
			doc:       `{"entries":[{"identifier":"urn:air:example.com:s:x","displayName":"x","type":"t/x","url":"https://example.com","data":{},"representativeQueries":["a","b"]}]}`,
			wantRules: []string{RuleSchema},
		},
		{
			name:      "neither url nor data",
			doc:       `{"entries":[{"identifier":"urn:air:example.com:s:x","displayName":"x","type":"t/x","representativeQueries":["a","b"]}]}`,
			wantRules: []string{RuleSchema},
		},
		{
			name:      "missing displayName",
			doc:       `{"entries":[{"identifier":"urn:air:example.com:s:x","type":"t/x","url":"https://example.com","representativeQueries":["a","b"]}]}`,
			wantRules: []string{RuleSchema},
		},
		{
			name:      "url not a uri",
			doc:       `{"entries":[{"identifier":"urn:air:example.com:s:x","displayName":"x","type":"t/x","url":"not a uri","representativeQueries":["a","b"]}]}`,
			wantRules: []string{RuleSchema},
		},
		{
			name:      "identifier off the schema pattern",
			doc:       `{"entries":[{"identifier":"urn:ai:example.com:s:x","displayName":"x","type":"t/x","url":"https://example.com","representativeQueries":["a","b"]}]}`,
			wantRules: []string{RuleIdentifier, RuleSchema},
		},
		{
			name:      "publisher is not an FQDN",
			doc:       `{"entries":[{"identifier":"urn:air:localhost:s:x","displayName":"x","type":"t/x","url":"https://example.com","representativeQueries":["a","b"]}]}`,
			wantRules: []string{RuleIdentifier},
		},
		{
			name:      "updatedAt not a date-time",
			doc:       `{"entries":[{"identifier":"urn:air:example.com:s:x","displayName":"x","type":"t/x","url":"https://example.com","updatedAt":"yesterday","representativeQueries":["a","b"]}]}`,
			wantRules: []string{RuleSchema},
		},
		{name: "no queries is a warning", doc: `{"entries":[` + noQueries + `]}`, wantRules: []string{RuleQueries}},
		{
			name:      "six queries is a warning",
			doc:       `{"entries":[{"identifier":"urn:air:example.com:s:x","displayName":"x","type":"t/x","url":"https://example.com","representativeQueries":["1","2","3","4","5","6"]}]}`,
			wantRules: []string{RuleQueries},
		},
		{
			name:      "duplicate identifier",
			doc:       `{"entries":[` + okEntry + `,` + okEntry + `]}`,
			wantRules: []string{RuleIdentifier},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			findings, err := Validate([]byte(tt.doc))

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRules, uniqueRules(findings), "%+v", findings)
		})
	}
}

func TestValidateSeverities(t *testing.T) {
	// Arrange
	doc := `{"entries":[` + noQueries + `]}`

	// Act
	findings, err := Validate([]byte(doc))

	// Assert
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, SeverityWarning, findings[0].Severity)
	assert.Equal(t, "urn:air:example.com:skills:x", findings[0].Identifier)
	assert.False(t, HasErrors(findings))
}

func TestValidateAcceptsTheUpstreamExample(t *testing.T) {
	// Arrange: conformance/examples/basic/ard.json at the pinned commit; the
	// two entries without representativeQueries are flagged as warnings there too.
	data, err := os.ReadFile(filepath.Join("testdata", "upstream-basic.json"))
	require.NoError(t, err)

	// Act
	findings, err := Validate(data)

	// Assert
	require.NoError(t, err)
	assert.False(t, HasErrors(findings), "%+v", findings)
	assert.Equal(t, []string{RuleQueries}, uniqueRules(findings))
}

func uniqueRules(findings []Finding) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range findings {
		if !seen[f.Rule] {
			seen[f.Rule] = true
			out = append(out, f.Rule)
		}
	}
	return out
}
