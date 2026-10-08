package gitutil

import (
	"strings"
	"testing"
)

func TestIsCommitSHA(t *testing.T) {
	sha1 := strings.Repeat("a1", 20)
	sha256 := strings.Repeat("0f", 32)
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"sha1", sha1, true},
		{"sha256 object format", sha256, true},
		{"uppercase", strings.ToUpper(sha1), false},
		{"39 chars", sha1[:39], false},
		{"41 chars", sha1 + "a", false},
		{"abbreviated", sha1[:7], false},
		{"non hex 40", strings.Repeat("g", 40), false},
		{"non hex 64", strings.Repeat("a", 63) + "z", false},
		{"63 chars", sha256[:63], false},
		{"branch", "main", false},
		{"empty", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsCommitSHA(c.in); got != c.want {
				t.Errorf("IsCommitSHA(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
