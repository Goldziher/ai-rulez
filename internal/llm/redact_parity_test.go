package llm_test

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// llm.RedactSecrets is the refuse-to-send check of the judge, the eval grader and the LLM
// verifiers. It missed credential shapes the security scan (AR001) catches, so a Google key or a
// private key block went to the model (RV-LLM-9). Both now share internal/secretpat.
func TestRedactSecretsShouldMaskEveryShapeTheSecurityScanCatches(t *testing.T) {
	tests := []struct {
		name, in string
	}{
		{"google api key", "key AIzaSyA1234567890abcdefghijklmnopqrstuv here"},
		{"github token", "ghp_0123456789abcdefghijABCDEFGHIJ012345"},
		{"github fine-grained token", "github_pat_11ABCDEFG0123456789_abcdefghijklmnop"},
		{"slack token", "xoxb-123456789012-abcdefghijkl"},
		{"private key block", "-----BEGIN RSA PRIVATE KEY-----\nMIIEow"},
		{"stripe live key", "sk_" + "live_" + "0123456789abcdefghijklmn"},
		{"json web token", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"},
		{"quoted generic credential", `password = "s3cr3tPassw0rdValue1234"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: the security scan masks it.
			if lint.RedactSecrets(tc.in) == tc.in {
				t.Fatalf("precondition: the security scan does not catch %q", tc.name)
			}

			// Act
			got := llm.RedactSecrets(tc.in)

			// Assert
			if got == tc.in {
				t.Errorf("llm.RedactSecrets left %q unmasked", tc.name)
			}
		})
	}
}
