package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSigningTrust_Reviewer(t *testing.T) {
	tests := []struct {
		name    string
		trust   SigningTrust
		wantErr string
	}{
		{name: "a key names its owner", trust: SigningTrust{Subject: "approval", KeyFile: "keys/a.pub", Reviewer: "alice@example.org"}},
		{name: "a github login", trust: SigningTrust{Subject: "approval", KeyFile: "keys/a.pub", Reviewer: "github:alice"}},
		{name: "an identity entry already is an identity", trust: SigningTrust{Identity: "alice@example.org", Issuer: "https://i", Reviewer: "alice@example.org"}, wantErr: "names the owner of a key_file"},
		{name: "surrounding space", trust: SigningTrust{KeyFile: "keys/a.pub", Reviewer: " alice "}, wantErr: "without surrounding space"},
		{name: "a control character", trust: SigningTrust{KeyFile: "keys/a.pub", Reviewer: "alice\nbob"}, wantErr: "control characters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := validateSigningTrust(tt.trust)

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
