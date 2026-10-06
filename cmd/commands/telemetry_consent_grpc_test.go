package commands

import (
	"bytes"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelemetryEnable_AcceptsTheDocumentedSchemelessGRPCEndpoint(t *testing.T) {
	// Arrange
	resetConsentFlags(t)
	env := setupTelemetry(t, "", "")
	telEnableEndpoint, telEnableProtocol = "collector.internal:4317", "grpc"
	var out bytes.Buffer

	// Act
	require.NoError(t, runTelemetryEnable(&out))

	// Assert: stored as https so a later resolve matches the record.
	record, err := telemetry.LoadConsent(consentFile(env))
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "https://collector.internal:4317", record.Endpoint)
	assert.Equal(t, "grpc", record.Protocol)
	assert.Contains(t, out.String(), "export is on")
}
