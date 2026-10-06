package signing

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func jwt(payload string) string {
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(`{"alg":"none"}`)) + "." + enc.EncodeToString([]byte(payload)) + ".sig"
}

func TestKeylessExpectedReviewer(t *testing.T) {
	tests := []struct {
		name   string
		token  string
		want   string
		wantOK bool
	}{
		{"a verified email", jwt(`{"iss":"https://accounts.google.com","email":"alice@example.org","email_verified":true}`), "alice@example.org", true},
		{"an unverified email is not what Fulcio certifies", jwt(`{"email":"alice@example.org","email_verified":false}`), "", false},
		{"no verification claim", jwt(`{"email":"alice@example.org"}`), "", false},
		{"a GitHub Actions workflow", jwt(`{"iss":"https://token.actions.githubusercontent.com","job_workflow_ref":"acme/config/.github/workflows/approve.yml@refs/heads/main"}`),
			"https://github.com/acme/config/.github/workflows/approve.yml@refs/heads/main", true},
		{"a workflow ref from another issuer is not trusted", jwt(`{"iss":"https://evil.example","job_workflow_ref":"x"}`), "", false},
		{"not a jwt", "opaque", "", false},
		{"a payload that is not json", jwt(`nope`), "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			signer, err := NewKeylessSigner(KeylessOptions{IDToken: tt.token})
			require.NoError(t, err)

			// Act
			got, ok := signer.ExpectedReviewer()

			// Assert
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestKeyExpectedReviewerMatchesTheSignedBundle(t *testing.T) {
	// Arrange
	priv, _, err := GenerateKeyPair(nil)
	require.NoError(t, err)
	ks, err := LoadKeySigner(priv, nil)
	require.NoError(t, err)

	// Act
	got, ok := ks.ExpectedReviewer()

	// Assert
	require.True(t, ok)
	assert.Equal(t, "key:"+ks.Key.Fingerprint(), got)
}
