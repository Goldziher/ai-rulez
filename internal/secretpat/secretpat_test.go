package secretpat

import (
	"strings"
	"testing"
)

// samples hold every credential shape plus near misses; each gate must agree
// with the pattern it guards on all of them.
func samples() []string {
	return []string{
		"AKIAIOSFODNN7EXAMPLE", "ASIAIOSFODNN7EXAMPLE", "akiaiosfodnn7example",
		"ghp_0123456789abcdefghijklmnopqrstuvwxyzAB", "gho_0123456789abcdefghijklmnopqrstuvwxyzAB", "ghx_0123456789abcdefghijklmnopqrstuvwxyzAB",
		"github_pat_0123456789abcdefghijklmnop", "xoxb-1234567890-abcdef", "AIzaSyA-0123456789abcdefghijklmnopqrstuv",
		"sk_" + "live_0123456789abcdefghijklmn", "rk_" + "live_0123456789abcdefghijklmn", "sk-ant-api03-abcdefghijklmnopqrstuv",
		"sk-proj-0123456789abcdefghijklmnopqrstuvwxyzABCD", "-----BEGIN RSA PRIVATE KEY-----", "-----BEGIN PRIVATE KEY-----",
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
