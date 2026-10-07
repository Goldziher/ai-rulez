package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateARD(t *testing.T) {
	tests := []struct {
		name string
		in   *ARDConfig
		want string
	}{
		{"absent", nil, ""},
		{"minimal", &ARDConfig{Publisher: "example.com", Namespace: "tools"}, ""},
		{"nested namespace", &ARDConfig{Publisher: "Example.COM", Namespace: "team:tools"}, ""},
		{"full", &ARDConfig{Publisher: "example.com", Namespace: "tools", BaseURL: "https://example.com/ard/", PluginType: "application/vnd.example.plugin+json"}, ""},
		{"no publisher", &ARDConfig{Namespace: "tools"}, "ard.publisher"},
		{"publisher not an FQDN", &ARDConfig{Publisher: "localhost", Namespace: "tools"}, "fully qualified"},
		{"publisher is an IP", &ARDConfig{Publisher: "10.0.0.1", Namespace: "tools"}, "IP address"},
		{"no namespace", &ARDConfig{Publisher: "example.com"}, "ard.namespace"},
		{"namespace segment", &ARDConfig{Publisher: "example.com", Namespace: "a b"}, "namespace segment"},
		{"empty namespace segment", &ARDConfig{Publisher: "example.com", Namespace: "a::b"}, "namespace segment"},
		{"http base url", &ARDConfig{Publisher: "example.com", Namespace: "t", BaseURL: "http://example.com"}, "https"},
		{"base url with credentials", &ARDConfig{Publisher: "example.com", Namespace: "t", BaseURL: "https://u:p@example.com"}, "https"},
		{"plugin type", &ARDConfig{Publisher: "example.com", Namespace: "t", PluginType: "plugin"}, "type/subtype"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&Config{ARD: tt.in}).validateARD()
			if tt.want == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestARDEmitterIsKnown(t *testing.T) {
	assert.Contains(t, PublishEmitterNames, PublishEmitterARD)
	require.NoError(t, (&Config{Publish: &PublishConfig{Emitters: []PublishEmitter{{Name: "ard"}}}}).validatePublish())
}
