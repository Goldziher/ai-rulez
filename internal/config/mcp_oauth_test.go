package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateMCPServerTypeAndOAuth(t *testing.T) {
	tests := []struct {
		name    string
		server  *MCPServer
		wantErr string
	}{
		{
			name:   "type alone is the transport",
			server: &MCPServer{Name: "s", Type: TransportHTTP, URL: "https://mcp.example.com"},
		},
		{
			name:   "type and transport that agree",
			server: &MCPServer{Name: "s", Type: TransportHTTP, Transport: TransportHTTP, URL: "https://mcp.example.com"},
		},
		{
			name:    "type and transport that differ",
			server:  &MCPServer{Name: "s", Type: TransportHTTP, Transport: TransportSSE, URL: "https://mcp.example.com"},
			wantErr: "sets transport",
		},
		{
			name:    "unknown type",
			server:  &MCPServer{Name: "s", Type: "grpc", URL: "https://mcp.example.com"},
			wantErr: "not a known transport",
		},
		{
			name: "oauth on a hosted server",
			server: &MCPServer{Name: "s", Type: TransportHTTP, URL: "https://mcp.example.com", OAuth: &MCPOAuth{
				ClientID: "cid", CallbackPort: 8080, Scopes: []string{"channels:read"},
			}},
		},
		{
			name:   "oauth with only a callback port is accepted (dynamic registration)",
			server: &MCPServer{Name: "s", Transport: TransportHTTP, URL: "https://mcp.example.com", OAuth: &MCPOAuth{CallbackPort: 8080}},
		},
		{
			name:    "oauth on a stdio server",
			server:  &MCPServer{Name: "s", Command: "npx", OAuth: &MCPOAuth{ClientID: "cid"}},
			wantErr: "requires transport http or sse",
		},
		{
			name:    "callback port out of range",
			server:  &MCPServer{Name: "s", Transport: TransportHTTP, URL: "https://x.test", OAuth: &MCPOAuth{CallbackPort: 70000}},
			wantErr: "between 1 and 65535",
		},
		{
			name:    "client id with a control character",
			server:  &MCPServer{Name: "s", Transport: TransportHTTP, URL: "https://x.test", OAuth: &MCPOAuth{ClientID: "a\nb"}},
			wantErr: "control characters",
		},
		{
			name:    "empty scope",
			server:  &MCPServer{Name: "s", Transport: TransportHTTP, URL: "https://x.test", OAuth: &MCPOAuth{ClientID: "c", Scopes: []string{"ok", "  "}}},
			wantErr: "empty scope",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &Config{MCPServers: map[string]*MCPServer{"s": tt.server}}

			// Act
			err := cfg.validateMCPServerTypeAndOAuth()

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

func TestMCPServerGetTransportPrefersType(t *testing.T) {
	tests := []struct {
		name   string
		server *MCPServer
		want   string
	}{
		{name: "type wins over transport", server: &MCPServer{Type: TransportHTTP, Transport: TransportSSE}, want: TransportHTTP},
		{name: "transport when type is unset", server: &MCPServer{Transport: TransportSSE}, want: TransportSSE},
		{name: "default stdio", server: &MCPServer{}, want: TransportStdio},
		{name: "nil server", server: nil, want: TransportStdio},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.server.GetTransport())
		})
	}
}

func TestMCPServerCloneCopiesOAuth(t *testing.T) {
	// Arrange
	original := MCPServer{Name: "s", OAuth: &MCPOAuth{ClientID: "cid", Scopes: []string{"a"}}}

	// Act
	clone := original.Clone()
	clone.OAuth.ClientID = "changed"
	clone.OAuth.Scopes[0] = "changed"

	// Assert
	assert.Equal(t, "cid", original.OAuth.ClientID)
	assert.Equal(t, []string{"a"}, original.OAuth.Scopes)
}
