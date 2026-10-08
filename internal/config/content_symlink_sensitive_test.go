package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSensitiveTarget(t *testing.T) {
	root := "/proj"
	tests := []struct {
		rel       string
		sensitive bool
	}{
		{".git/config", true},
		{".Git/config", true},
		{".GIT/HEAD", true},
		{"sub/.gIt/hooks/x", true},
		{".gitignore", false},
		{"docs/guide.md", false},
		{".ai-rulez/config.local.toml", true},
		{".ai-rulez/config.local.yaml", true},
		{".ai-rulez/config.toml", false},
		{".docker/config.json", true},
		{"config.json", false},
		{".kube/config", true},
		{"infra/.KUBE/config", true},
		{"config", false},
		{"terraform.tfstate", true},
		{"infra/prod.tfstate.backup", true},
		{".npmrc", true},
		{".pypirc", true},
		{".netrc", true},
		{"keys/id_rsa", true},
		{"keys/id_rsa_deploy", true},
		{"keys/id_ed25519_work", true},
		{"keys/id_rsa.pub", false},
		{".aws/credentials", true},
	}
	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			got := sensitiveTarget(root, root+"/"+tt.rel)
			assert.Equal(t, tt.sensitive, got != "", got)
		})
	}
}
