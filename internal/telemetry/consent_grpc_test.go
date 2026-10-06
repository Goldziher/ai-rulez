package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolve_SchemelessGRPCEndpointIsHTTPSAndMatchesItsRecord(t *testing.T) {
	// Arrange: consent recorded for the normalised form, endpoint given as host:port.
	consent := record("https://collector.internal:4317", "grpc", false, false)

	// Act
	s := Resolve(Layers{Consent: consent, Getenv: env(EnvEndpoint, "collector.internal:4317", EnvProtocol, "grpc")})

	// Assert
	assert.Equal(t, "https://collector.internal:4317", s.Endpoint)
	assert.Equal(t, ConsentRecord, s.ConsentState)
	assert.Empty(t, s.Problems)
	assert.True(t, s.ExportActive())
}
