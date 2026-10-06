package signing

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// ReviewerHinter is a Signer that can say, before anything is signed or logged,
// which reviewer identity its signature will carry (the identity ApprovalReviewer
// reads back from the bundle). `approve --sign` uses it to check the reviewer
// allowlist before a keyless signature is written to a public transparency log.
type ReviewerHinter interface {
	// ExpectedReviewer is the identity of the signature to come; false when it
	// cannot be told from what the signer holds.
	ExpectedReviewer() (string, bool)
}

// ExpectedReviewer implements ReviewerHinter: "key:<fingerprint>".
func (k *KeySigner) ExpectedReviewer() (string, bool) {
	fp := k.Key.Fingerprint()
	return "key:" + fp, fp != ""
}

// githubActionsIssuer is the OIDC issuer of GitHub Actions identity tokens.
const githubActionsIssuer = "https://token.actions.githubusercontent.com"

// ExpectedReviewer implements ReviewerHinter. Fulcio puts the token's verified
// email into the certificate, and for GitHub Actions the workflow reference as a
// URL; the claims are read here without verifying the token (Fulcio does that),
// only to learn what the certificate will name. Any other token shape is unknown.
func (k *KeylessSigner) ExpectedReviewer() (string, bool) {
	claims, ok := tokenClaims(k.opts.IDToken)
	if !ok {
		return "", false
	}
	switch {
	case claims.Email != "" && claims.EmailVerified != nil && *claims.EmailVerified:
		return claims.Email, true
	case claims.Issuer == githubActionsIssuer && claims.JobWorkflowRef != "":
		return "https://github.com/" + claims.JobWorkflowRef, true
	}
	return "", false
}

type idTokenClaims struct {
	Issuer         string `json:"iss"`
	Email          string `json:"email"`
	EmailVerified  *bool  `json:"email_verified"`
	JobWorkflowRef string `json:"job_workflow_ref"`
}

// tokenClaims decodes the payload of a JWT; it never trusts it.
func tokenClaims(token string) (idTokenClaims, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return idTokenClaims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return idTokenClaims{}, false
	}
	var c idTokenClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		return idTokenClaims{}, false
	}
	return c, true
}
