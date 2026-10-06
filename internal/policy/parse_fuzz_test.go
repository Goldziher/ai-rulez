package policy

import (
	"strings"
	"testing"
)

// FuzzParseDoc: whatever the bytes, parsing never panics, a rejected file is an
// AR743, and an accepted one folds without loosening itself and can be shown.
func FuzzParseDoc(f *testing.F) {
	for _, seed := range []string{
		"policy_version = 1\n",
		"policy_version = 1\nextends = [\"a.toml\", \"https://x.example/p.toml@sha256:" + strings.Repeat("a", 64) + "\"]\n",
		"policy_version = 1\n[sources]\nallowed_hosts = [\"github.com/example-org\"]\nmin_release_age = \"7d\"\n",
		"policy_version = 1\n[lint]\nno_inline_ignore = [\"AR001\"]\n[lint.max_findings]\nAR001 = 0\n[lint.budgets.skill]\nmax_tokens = 100\n",
		"policy_version = 1\n[signing]\ntlog = \"required\"\nmax_age = \"90d\"\n[[signing.trust]]\nidentity = \"a\"\nissuer = \"i\"\n",
		"policy_version = 1\n[mcp]\nallowed_commands = [\"npx\"]\ndeny_transports = [\"http\"]\n[hooks]\nallow = false\n",
		"policy_version = 1\n[governance]\nmin_approvers = 2\napprovers = [\"a@x.org\"]\n",
		"policy_version = 2\n", "", "not toml [", "policy_version = 1\n[lint]\nrequired_codes = [\"nope\"]\n",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := parseDoc("fuzz.toml", data)
		if err != nil {
			if !strings.Contains(err.Error(), "AR743") {
				t.Fatalf("a rejected policy must be AR743: %v", err)
			}
			return
		}
		p := d.policy
		if got := Merge(p, Policy{}); got.Sources.MinReleaseAge != p.Sources.MinReleaseAge {
			t.Fatalf("the zero policy changed the release age: %v vs %v", got.Sources.MinReleaseAge, p.Sources.MinReleaseAge)
		}
		if bad := Loosens(p, p); len(bad) != 0 {
			t.Fatalf("a policy loosens itself: %v", bad)
		}
		if bad := Loosens(p, Merge(p, p)); len(bad) != 0 {
			t.Fatalf("the fold of a policy with itself loosens it: %v", bad)
		}
		res := Resolve([]Layer{{Origin: OriginFlag, Path: "fuzz.toml", Digest: "sha256:" + strings.Repeat("0", 64), Policy: p}})
		var sb strings.Builder
		BuildReport(res, nil).WriteText(&sb)
	})
}
