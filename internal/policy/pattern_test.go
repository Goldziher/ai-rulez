package policy

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePattern(t *testing.T) {
	tests := []struct {
		name, in, want string
		wantErr        string
	}{
		{"host", "GitHub.com", "github.com", ""},
		{"host and org", "github.com/Example-Org", "github.com/example-org", ""},
		{"subdomain wildcard", "*.example.org", "*.example.org", ""},
		{"trailing double star is a prefix", "github.com/org/**", "github.com/org", ""},
		{"prefix glob segment", "github.com/example-*", "github.com/example-*", ""},
		{"empty", "  ", "", "empty"},
		{"scheme", "https://github.com", "", "not a host pattern"},
		{"bare star host", "*", "", "concrete host"},
		{"mid host wildcard", "git*.example.org", "", "leading *."},
		{"double star mid path", "github.com/**/rules", "", "last segment"},
		{"two stars in a segment", "github.com/a*b*", "", "one * at its start or end"},
		{"star inside a segment", "github.com/a*b", "", "one * at its start or end"},
		{"empty segment", "github.com//x", "", "empty path segment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, err := normalizePattern(tt.in)
			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSplitLocation(t *testing.T) {
	tests := []struct {
		source   string
		wantHost string
		wantPath string
		remote   bool
	}{
		{"https://github.com/Example-Org/rules.git", "github.com", "example-org/rules", true},
		{"git@github.com:example-org/rules.git", "github.com", "example-org/rules", true},
		{"ssh://git@git.example.org:2222/team/rules", "git.example.org", "team/rules", true},
		{"git+https://user:pw@host.example/org/repo/", "host.example", "org/repo", true},
		{"file:///tmp/repo", "", "", false},
		{"../shared", "", "", false},
		{"/abs/path", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			host, segs, ok := splitLocation(tt.source)
			assert.Equal(t, tt.remote, ok)
			assert.Equal(t, tt.wantHost, host)
			assert.Equal(t, tt.wantPath, strings.Join(segs, "/"))
		})
	}
}

func TestMatches(t *testing.T) {
	tests := []struct {
		pattern, host, path string
		want                bool
	}{
		{"github.com", "github.com", "any/thing", true},
		{"github.com/example-org", "github.com", "example-org/repo", true},
		{"github.com/example-org", "github.com", "example-orgx/repo", false},
		{"github.com/example-org", "github.com", "", false},
		{"github.com/example-*", "github.com", "example-team/repo", true},
		{"github.com/*/rules", "github.com", "a/rules/x", true},
		{"github.com/*/rules", "github.com", "a/other", false},
		{"*.example.org", "git.example.org", "x", true},
		{"*.example.org", "example.org", "x", true},
		{"*.example.org", "badexample.org", "x", false},
		{"github.com", "gitlab.com", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+" "+tt.host+"/"+tt.path, func(t *testing.T) {
			p, err := parsePattern(tt.pattern)
			require.NoError(t, err)
			var segs []string
			if tt.path != "" {
				segs = strings.Split(tt.path, "/")
			}
			assert.Equal(t, tt.want, p.matches(tt.host, segs))
		})
	}
}

func TestCovers(t *testing.T) {
	tests := []struct {
		name, policy, repo string
		want               bool
	}{
		{"equal", "github.com", "github.com", true},
		{"narrower path under a host", "github.com", "github.com/example-org", true},
		{"wider host than policy path", "github.com/example-org", "github.com", false},
		{"same org", "github.com/example-org", "github.com/example-org/rules", true},
		{"other org", "github.com/example-org", "github.com/other", false},
		{"literal does not cover a wildcard", "github.com/example-org", "github.com/*", false},
		{"star covers a literal", "github.com/*", "github.com/example-org", true},
		{"star covers a star", "github.com/*", "github.com/*", true},
		{"prefix glob covers a longer prefix", "github.com/example-*", "github.com/example-team-*", true},
		{"prefix glob covers a literal", "github.com/example-*", "github.com/example-team", true},
		{"prefix glob does not cover a suffix glob", "github.com/example-*", "github.com/*-team", false},
		{"suffix glob covers a literal", "github.com/*-team", "github.com/a-team", true},
		{"wildcard host covers subdomain", "*.example.org", "git.example.org", true},
		{"wildcard host covers itself", "*.example.org", "*.example.org", true},
		{"wildcard host covers a narrower wildcard", "*.example.org", "*.git.example.org", true},
		{"literal host does not cover a wildcard host", "example.org", "*.example.org", false},
		{"wildcard host does not cover a lookalike", "*.example.org", "badexample.org", false},
		{"unparsable policy covers nothing", "https://x", "github.com", false},
		{"unparsable repo is covered by nothing", "github.com", "*", false},
		{"trailing ** is the prefix", "github.com/org/**", "github.com/org/repo", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Covers(tt.policy, tt.repo))
		})
	}
}

// universe is every location over a small alphabet, so a pattern's match set
// can be computed by brute force.
func universe() (hosts []string, paths [][]string) {
	hosts = []string{"a.com", "b.com", "x.a.com", "y.x.a.com", "a", "xa.com"}
	segs := []string{"a", "b", "ab", "ba", "aab"}
	paths = [][]string{nil}
	for _, s1 := range segs {
		paths = append(paths, []string{s1})
		for _, s2 := range segs {
			paths = append(paths, []string{s1, s2})
			for _, s3 := range segs[:2] {
				paths = append(paths, []string{s1, s2, s3})
			}
		}
	}
	return hosts, paths
}

func randomPattern(rng *rand.Rand) string {
	hosts := []string{"a.com", "b.com", "*.a.com", "*.x.a.com", "*.com", "x.a.com", "a"}
	segs := []string{"a", "b", "ab", "*", "a*", "*b", "ab*", "*ab", "ba", "aa*", "**"}
	out := hosts[rng.Intn(len(hosts))]
	for n := rng.Intn(4); n > 0; n-- {
		out += "/" + segs[rng.Intn(len(segs))]
	}
	return out
}

// assertNoFalseAccept checks the safety property of Covers over the universe:
// when policy covers repo, every location repo matches is matched by policy.
func assertNoFalseAccept(t *testing.T, policy, repo string, hosts []string, paths [][]string) bool {
	t.Helper()
	if !Covers(policy, repo) {
		return false
	}
	pp, err := parsePattern(policy)
	require.NoError(t, err)
	rp, err := parsePattern(repo)
	require.NoError(t, err)
	for _, h := range hosts {
		for _, p := range paths {
			if rp.matches(h, p) && !pp.matches(h, p) {
				t.Fatalf("Covers(%q, %q) accepted, but %s/%s is matched by the repo pattern only", policy, repo, h, strings.Join(p, "/"))
			}
		}
	}
	return true
}

func TestCoversNeverFalselyAccepts(t *testing.T) {
	// Arrange
	hosts, paths := universe()
	rng := rand.New(rand.NewSource(216))
	accepted := 0
	// Act and Assert
	for i := 0; i < 4000; i++ {
		p, r := randomPattern(rng), randomPattern(rng)
		if _, err := parsePattern(p); err != nil {
			continue
		}
		if _, err := parsePattern(r); err != nil {
			continue
		}
		if assertNoFalseAccept(t, p, r, hosts, paths) {
			accepted++
		}
	}
	assert.Greater(t, accepted, 100, "the generator must reach the accepting branch")
}

func FuzzCovers(f *testing.F) {
	for _, seed := range [][2]string{
		{"github.com", "github.com/org"}, {"*.a.com", "x.a.com/a"}, {"a.com/*", "a.com/ab"},
		{"a.com/a*", "a.com/ab*"}, {"a.com/*b", "a.com/*ab"}, {"*.com", "*.a.com/b"},
	} {
		f.Add(seed[0], seed[1])
	}
	hosts, paths := universe()
	f.Fuzz(func(t *testing.T, policy, repo string) {
		if _, err := parsePattern(policy); err != nil {
			return
		}
		if _, err := parsePattern(repo); err != nil {
			return
		}
		assertNoFalseAccept(t, policy, repo, hosts, paths)
	})
}
