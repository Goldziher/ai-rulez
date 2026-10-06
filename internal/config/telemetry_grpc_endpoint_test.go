package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeTelemetryEndpoint(t *testing.T) {
	tests := []struct {
		name, endpoint, protocol, want string
	}{
		{name: "grpc host and port gets https", endpoint: "collector.internal:4317", protocol: "grpc", want: "https://collector.internal:4317"},
		{name: "grpc bare host gets https", endpoint: "collector.internal", protocol: "grpc", want: "https://collector.internal"},
		{name: "grpc keeps an explicit scheme", endpoint: "http://127.0.0.1:4317", protocol: "grpc", want: "http://127.0.0.1:4317"},
		{name: "http protocols are never rewritten", endpoint: "collector.internal:4318", protocol: "http/json", want: "collector.internal:4318"},
		{name: "empty stays empty", endpoint: "", protocol: "grpc", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeTelemetryEndpoint(tt.endpoint, tt.protocol))
		})
	}
}

func TestTelemetryConfig_ValidateAcceptsTheDocumentedGRPCForm(t *testing.T) {
	tests := []struct {
		name string
		cfg  TelemetryConfig
		want []string
	}{
		{name: "host:port with grpc", cfg: TelemetryConfig{OTLPEndpoint: "collector.internal:4317", OTLPProtocol: "grpc"}},
		{name: "host:port over http is still not a URL", cfg: TelemetryConfig{OTLPEndpoint: "collector.internal:4318", OTLPProtocol: "http/json"}, want: []string{"telemetry.otlp_endpoint: not a URL with a host"}},
		{name: "a grpc path is refused after normalising", cfg: TelemetryConfig{OTLPEndpoint: "collector.internal:4317/x", OTLPProtocol: "grpc"}, want: []string{"telemetry.otlp_endpoint: a grpc endpoint names host[:port] only, without a path"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.Validate())
		})
	}
}
