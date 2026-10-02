package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateMCPServerHeaders(t *testing.T) {
	tests := []struct {
		name      string
		transport string
		headers   map[string]string
		wantErr   string
	}{
		{name: "http with headers", transport: TransportHTTP, headers: map[string]string{"Authorization": "Bearer ${TOKEN}"}},
		{name: "sse with headers", transport: TransportSSE, headers: map[string]string{"X-Api-Key": "k"}},
		{name: "stdio without headers", transport: ""},
		{name: "stdio with headers", transport: "", headers: map[string]string{"X-A": "b"}, wantErr: "require transport"},
		{name: "invalid header name", transport: TransportHTTP, headers: map[string]string{"Bad Header": "v"}, wantErr: "invalid header name"},
		{name: "empty header name", transport: TransportHTTP, headers: map[string]string{"": "v"}, wantErr: "invalid header name"},
		{name: "header value with newline", transport: TransportHTTP, headers: map[string]string{"X-A": "v\r\nX-B: injected"}, wantErr: "must not contain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &Config{MCPServers: map[string]*MCPServer{
				"s": {Name: "s", Transport: tt.transport, URL: "https://mcp.example.com", Command: "x", Headers: tt.headers},
			}}

			// Act
			err := cfg.validateMCPServerHeaders()

			// Assert
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
