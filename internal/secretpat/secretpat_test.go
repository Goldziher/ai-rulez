package secretpat

import (
	"strings"
	"testing"
)

func TestBuiltinMatchesEachShape(t *testing.T) {
	tests := []struct {
		name, in string
	}{
		{"AWS access key id", "AKIA" + strings.Repeat("A1", 8)},
		{"GitHub token", "ghp_" + strings.Repeat("a1", 18)},
		{"GitHub fine-grained token", "github_pat_" + strings.Repeat("aB1_", 6)},
		{"npm access token", "npm_" + strings.Repeat("aZ9", 12)},
		{"Slack token", "xoxb-123456789012-abcdefghijkl"},
		{"Anthropic API key", "sk-ant-" + strings.Repeat("a1", 12)},
		{"OpenAI API key", "sk-proj-" + strings.Repeat("a1", 24)},
		{"private key block", "-----BEGIN RSA PRIVATE KEY-----"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Redact("x "+tt.in+" y", "[R]"); got == "x "+tt.in+" y" {
				t.Errorf("%q was not recognized", tt.in)
			}
		})
	}
}

// Detection is stricter than redaction by design: a short token-looking string is
// not a finding, but the redactors in front of a model still hide it.
func TestBuiltinRequiresRealTokenLengths(t *testing.T) {
	short := []string{
		"ghp_" + strings.Repeat("a", 25),
		"npm_" + strings.Repeat("a", 20),
		"sk-" + strings.Repeat("a", 12),
	}
	for _, in := range short {
		if got := Redact(in, "[R]"); got != in {
			t.Errorf("%q is below the detection threshold but was flagged: %q", in, got)
		}
	}
}
