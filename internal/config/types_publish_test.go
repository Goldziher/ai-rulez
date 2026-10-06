package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePublish(t *testing.T) {
	tests := []struct {
		name string
		in   *PublishConfig
		want string
	}{
		{"absent", nil, ""},
		{"full", &PublishConfig{
			Runtimes: []string{"claude", "cursor"}, RequireSignature: true,
			OCI:         &PublishOCI{Ref: "ghcr.io/acme/skills/conventions"},
			NPM:         &PublishNPM{Scope: "@acme", Access: "public", Registry: "https://npm.example.com"},
			Marketplace: &PublishMarketplace{Channels: map[string]string{"canary": "main"}},
			Emitters:    []PublishEmitter{{Name: "port"}, {Name: "template", Template: "tools/x.tmpl", Output: "x.json"}},
		}, ""},
		{"unknown runtime", &PublishConfig{Runtimes: []string{"vim"}}, "unknown runtime"},
		{"oci ref with a tag", &PublishConfig{OCI: &PublishOCI{Ref: "ghcr.io/acme/skills:1.0"}}, "repository reference"},
		{"oci ref upper case", &PublishConfig{OCI: &PublishOCI{Ref: "ghcr.io/Acme/skills"}}, "repository reference"},
		{"oci ref without host", &PublishConfig{OCI: &PublishOCI{Ref: "skills"}}, "repository reference"},
		{"npm scope without at", &PublishConfig{NPM: &PublishNPM{Scope: "acme"}}, "npm scope"},
		{"npm access", &PublishConfig{NPM: &PublishNPM{Access: "world"}}, "invalid access"},
		{"npm http registry", &PublishConfig{NPM: &PublishNPM{Registry: "http://npm.example.com"}}, "https"},
		{"channel name", &PublishConfig{Marketplace: &PublishMarketplace{Channels: map[string]string{"Bad Name": "main"}}}, "invalid channel name"},
		{"channel ref", &PublishConfig{Marketplace: &PublishMarketplace{Channels: map[string]string{"canary": "../x"}}}, "invalid git ref"},
		{"unknown emitter", &PublishConfig{Emitters: []PublishEmitter{{Name: "jira"}}}, "unknown emitter"},
		{"template emitter without a file", &PublishConfig{Emitters: []PublishEmitter{{Name: "template"}}}, "needs a template"},
		{"template escapes", &PublishConfig{Emitters: []PublishEmitter{{Name: "template", Template: "../x.tmpl"}}}, "inside the project"},
		{"template absolute", &PublishConfig{Emitters: []PublishEmitter{{Name: "template", Template: "/etc/x.tmpl"}}}, "inside the project"},
		{"template output path", &PublishConfig{Emitters: []PublishEmitter{{Name: "template", Template: "x.tmpl", Output: "a/b"}}}, "plain file name"},
		{"template on a vendor emitter", &PublishConfig{Emitters: []PublishEmitter{{Name: "port", Template: "x.tmpl"}}}, "emitter only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&Config{Publish: tt.in}).validatePublish()

			if tt.want == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
