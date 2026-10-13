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

const awsSecret = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"

func TestAWSSecretAccessKeyIsContextAnchored(t *testing.T) {
	tests := []struct {
		name, in, wantValue string
	}{
		{"env assignment", "export AWS_SECRET_ACCESS_KEY=" + awsSecret, awsSecret},
		{"lower case quoted with spaces", `aws_secret_access_key = "` + awsSecret + `"`, awsSecret},
		{"single quoted", "aws_secret_access_key='" + awsSecret + "'", awsSecret},
		{"yaml colon", "AWS_SECRET_ACCESS_KEY: " + awsSecret, awsSecret},
		{"json key", `{"aws_secret_access_key": "` + awsSecret + `"}`, awsSecret},
		{"mixed case prefixed name", "My_Aws_Secret_Access_Key=" + awsSecret + " # prod", awsSecret},
		{"value ends in a base64 symbol", "AWS_SECRET_ACCESS_KEY=" + awsSecret[:39] + "+", awsSecret[:39] + "+"},
		{"bare 40 characters", awsSecret, ""},
		{"40 characters under another name", "checksum=" + awsSecret, ""},
		{"generic secret name", "SECRET_KEY=" + awsSecret, ""},
		{"too short", "AWS_SECRET_ACCESS_KEY=" + awsSecret[:39], ""},
		{"too long", "AWS_SECRET_ACCESS_KEY=" + awsSecret + "A", ""},
		{"environment lookup", "AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY}", ""},
		{"placeholder", "AWS_SECRET_ACCESS_KEY=<your-secret-access-key-here>", ""},
		{"name only", "set AWS_SECRET_ACCESS_KEY before running", ""},
	}
	var pat Pattern
	for _, p := range Builtin {
		if p.Name == "AWS secret access key" {
			pat = p
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ""
			if pat.MayMatch(tt.in) {
				got = FindValue(pat.Re, tt.in)
			}
			if got != tt.wantValue {
				t.Fatalf("FindValue(%q) = %q, want %q", tt.in, got, tt.wantValue)
			}
			red := Redact(tt.in, "[R]")
			if tt.wantValue == "" && red != tt.in {
				t.Errorf("Redact changed %q to %q", tt.in, red)
			}
			if tt.wantValue != "" && (strings.Contains(red, tt.wantValue) || !strings.Contains(red, "[R]")) {
				t.Errorf("Redact left the value in %q", red)
			}
		})
	}
}

func TestRedactKeepsTheNameAndTrailingCharacter(t *testing.T) {
	got := Redact(`aws_secret_access_key = "`+awsSecret+`", next`, "[R]")
	if want := `aws_secret_access_key = "[R]", next`; got != want {
		t.Errorf("Redact = %q, want %q", got, want)
	}
}
