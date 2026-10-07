package llm

import (
	"strings"
	"testing"
)

func TestRedactSecretsCoversCommonCredentialShapes(t *testing.T) {
	for name, secret := range map[string]string{
		"github":     "ghp_abcdefghijklmnopqrstuvwxyz0123456789",
		"github-pat": "github_pat_11ABCDEFG0abcdefghijklmnop",
		"gitlab":     "glpat-abcdefghijklmnopqrst",
		"google":     "AIzaSyA-abcdefghijklmnopqrstuvwxyz01234",
		"slack":      "xoxb-1234567890-abcdefghij",
		"jwt":        "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abcdefghijk",
		"pem":        "-----BEGIN RSA PRIVATE KEY-----",
		"basic":      "Basic dXNlcjpwYXNzd29yZA==",
		"aws-temp":   "ASIAABCDEFGHIJKLMNOP",
	} {
		if got := RedactSecrets("error: " + secret + " rejected"); strings.Contains(got, secret) || !strings.Contains(got, "[REDACTED]") {
			t.Errorf("%s was not redacted: %q", name, got)
		}
	}
	if got := RedactSecrets("model gpt-4o-mini is rate limited"); got != "model gpt-4o-mini is rate limited" {
		t.Errorf("plain text changed: %q", got)
	}
}
