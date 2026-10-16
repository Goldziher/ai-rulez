package secretpat

import (
	"strings"
	"testing"
)

// samples hold every credential shape plus near misses; each gate must agree
// with the pattern it guards on all of them.
func samples() []string {
	return []string{
		"export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "Aws_Secret_Access_Key: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"AKIAIOSFODNN7EXAMPLE", "ASIAIOSFODNN7EXAMPLE", "akiaiosfodnn7example",
		"ghp_0123456789abcdefghijklmnopqrstuvwxyzAB", "gho_0123456789abcdefghijklmnopqrstuvwxyzAB", "ghx_0123456789abcdefghijklmnopqrstuvwxyzAB",
		"github_pat_0123456789abcdefghijklmnop", "xoxb-1234567890-abcdef", "AIzaSyA-0123456789abcdefghijklmnopqrstuv",
		"sk_" + "live_0123456789abcdefghijklmn", "rk_" + "live_0123456789abcdefghijklmn", "sk-ant-api03-abcdefghijklmnopqrstuv",
		"sk-proj-0123456789abcdefghijklmnopqrstuvwxyzABCD", "-----BEGIN " + "RSA PRIVATE KEY-----", "-----BEGIN " + "PRIVATE KEY-----",
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abcdefghijklmnop",
		`password = "Sup3rS3cretValue99xyz"`, `API_KEY: 'abcdefghij1234567890'`, `ApiKey="abcdefghij1234567890"`, `TOKEN="abcdefghij1234567890"`,
		`client-secret = "abcdefghij1234567890"`, `PaSsWd: "abcdefghij1234567890"`, `ſecret = "abcdefghij1234567890"`,
		`toKen = "abcdefghij1234567890"`, "plain prose with no credential at all", "", "sk-", "ghp_", "AKIA",
		strings.Repeat("x", 300) + " token='" + strings.Repeat("a1", 20) + "'",
	}
}

func TestStemsNeverRejectAMatch(t *testing.T) {
	for _, text := range samples() {
		for _, p := range Builtin {
			if p.Re.MatchString(text) && !p.MayMatch(text) {
				t.Errorf("%s: gate rejects %q, which the pattern matches", p.Name, text)
			}
		}
		if GenericCredential.MatchString(text) && !MayHaveGenericCredential(text) {
			t.Errorf("generic credential gate rejects %q, which the pattern matches", text)
		}
	}
}

func TestStemsRejectOrdinaryProse(t *testing.T) {
	prose := "Use the project conventions when you write the change, keep the diff small."
	for _, p := range Builtin {
		if p.MayMatch(prose) {
			t.Errorf("%s: gate passes ordinary prose", p.Name)
		}
	}
	if MayHaveGenericCredential(prose) {
		t.Error("generic credential gate passes ordinary prose")
	}
}

func TestRedactMasksEveryShape(t *testing.T) {
	for _, text := range samples() {
		got := Redact(text, "[X]")
		matched := GenericCredential.MatchString(text)
		for _, p := range Builtin {
			matched = matched || p.Re.MatchString(text)
		}
		if matched && got == text && !GenericCredential.MatchString(text) {
			t.Errorf("Redact left %q unchanged", text)
		}
		if !matched && got != text {
			t.Errorf("Redact changed %q to %q without a match", text, got)
		}
	}
}

func TestIsCredentialValue(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"saq_tn3270_scan_password", false},
		{"saq_ssh_scan_password", false},
		{"saq_windows_scan_kdc_host", false},
		{"Xk9fQ2pLm4Zr8TvB1nWc", true},
		{"ab12_cd34_ef56_gh78_ij90", true},
		{"two_words9x_only_1", true},
		{"sk_live_a1b2c3d4e5f6g7h8i9j0", true},
		{"nodigitsatall", false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := IsCredentialValue(tt.value); got != tt.want {
				t.Errorf("IsCredentialValue(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
