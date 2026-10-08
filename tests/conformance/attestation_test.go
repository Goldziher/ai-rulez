package conformance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/signing"
	"github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
)

// Neither in-toto nor DSSE publishes a JSON Schema, so schemas/in-toto and
// schemas/dsse hold the shape transcribed from the specifications. The test
// signs offline with a throwaway key and checks the envelope inside the
// Sigstore bundle and the statement it carries.
func TestSignedStatementIsADSSEEnvelopeOverAnInTotoStatement(t *testing.T) {
	// Arrange
	envelopeSchema := compile(t, "https://ai-rulez.dev/conformance/dsse-envelope.schema.json",
		map[string]string{"https://ai-rulez.dev/conformance/dsse-envelope.schema.json": "dsse/envelope.schema.json"})
	statementSchema := compile(t, "https://ai-rulez.dev/conformance/in-toto-statement-v1.schema.json",
		map[string]string{"https://ai-rulez.dev/conformance/in-toto-statement-v1.schema.json": "in-toto/statement-v1.schema.json"})
	priv, _, err := sigstore.GenerateKeyPair(nil)
	require.NoError(t, err)
	signer, err := sigstore.LoadKeySigner(priv, nil)
	require.NoError(t, err)
	digestHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	statement, err := signing.NewStatement(signing.PredicateLock,
		[]signing.Subject{{Name: "ai-rulez.lock", Digest: map[string]string{"sha256": digestHex}}},
		map[string]string{"note": "conformance"})
	require.NoError(t, err)

	// Act
	bundle, err := signing.SignStatement(context.Background(), signer, statement)
	require.NoError(t, err)

	// Assert
	var doc struct {
		MediaType      string          `json:"mediaType"`
		DSSEEnvelope   json.RawMessage `json:"dsseEnvelope"`
		VerifyMaterial json.RawMessage `json:"verificationMaterial"`
	}
	require.NoError(t, json.Unmarshal(bundle, &doc))
	assert.Contains(t, doc.MediaType, "application/vnd.dev.sigstore.bundle")
	requireValid(t, envelopeSchema, doc.DSSEEnvelope)

	var envelope struct {
		Payload     string `json:"payload"`
		PayloadType string `json:"payloadType"`
	}
	require.NoError(t, json.Unmarshal(doc.DSSEEnvelope, &envelope))
	assert.Equal(t, "application/vnd.in-toto+json", envelope.PayloadType)
	payload, err := base64.StdEncoding.DecodeString(envelope.Payload)
	require.NoError(t, err)
	requireValid(t, statementSchema, payload)

	requireInvalid(t, envelopeSchema, `{"payloadType":"x","payload":"AAAA","signatures":[]}`)
	requireInvalid(t, statementSchema, `{"_type":"https://in-toto.io/Statement/v1","subject":[],"predicateType":"https://x.example/p","predicate":{}}`)
}
