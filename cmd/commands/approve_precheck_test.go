package commands

import (
	"context"
	"testing"

	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

// countingSigner records that something was signed (for keyless, logged) and
// reports the reviewer identity it would carry.
type countingSigner struct {
	who    string
	known  bool
	signed int
}

func (c *countingSigner) Bundle(context.Context, signing.Content) (*protobundle.Bundle, error) {
	c.signed++
	return nil, assert.AnError
}

func (c *countingSigner) ExpectedReviewer() (string, bool) { return c.who, c.known }

func TestSignedAttesterChecksTheAllowlistBeforeSigning(t *testing.T) {
	tests := []struct {
		name    string
		extra   string
		signer  *countingSigner
		wantErr string
	}{
		{name: "a reviewer outside the allowlist is refused unsigned", extra: "\napprovers = [\"alice@example.org\"]\n",
			signer: &countingSigner{who: "mallory@example.org", known: true}, wantErr: "is not in [governance] approvers"},
		{name: "an allowed reviewer reaches the signer", extra: "\napprovers = [\"alice@example.org\"]\n",
			signer: &countingSigner{who: "Alice@Example.org", known: true}, wantErr: "sign the attestation"},
		{name: "an unknown identity under an allowlist is refused unsigned", extra: "\napprovers = [\"alice@example.org\"]\n",
			signer: &countingSigner{}, wantErr: "cannot tell which identity"},
		{name: "an unknown identity without a restriction is left to the check after signing", signer: &countingSigner{}, wantErr: "sign the attestation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			approveProject(t, tt.extra)
			env, err := loadApproveEnv()
			require.NoError(t, err)
			subs, err := env.resolveAll([]string{"rule:style"})
			require.NoError(t, err)
			attester := signedAttester{signer: tt.signer, precheck: env.precheckSigner}

			// Act
			_, err = attester.drafts(context.Background(), subs[0], draftInput{})

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			wantSigned := 0
			if tt.wantErr == "sign the attestation" {
				wantSigned = 1
			}
			assert.Equal(t, wantSigned, tt.signer.signed, "nothing is signed (logged) before the allowlist passes")
		})
	}
}
