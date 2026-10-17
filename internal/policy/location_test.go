package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestSplitLocationParsesTheHostTheTransportUses(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		wantHost string
		wantSegs []string
	}{
		{"trailing dot is the same host", "https://GitHub.com./Evil/repo", "github.com", []string{"evil", "repo"}},
		{"trailing dot with a port", "https://github.com.:8443/o/r", "github.com", []string{"o", "r"}},
		{"scp-like trailing dot", "git@github.com.:o/r.git", "github.com", []string{"o", "r"}},
		{"uppercase host and path", "HTTPS://GitHub.COM/Org/Repo.GIT", "github.com", []string{"org", "repo"}},
		{"port is dropped", "https://github.com:8443/org/repo.git", "github.com", []string{"org", "repo"}},
		{"query before an at sign names the real host", "https://evil.example?@github.com/org/repo.git", "evil.example", nil},
		{"fragment before an at sign names the real host", "https://evil.example#@github.com/org/repo.git", "evil.example", nil},
		{"userinfo is not the host", "https://github.com@evil.example/org/repo.git", "evil.example", []string{"org", "repo"}},
		{"user and password are not the host", "https://user:pw@evil.example:443/org/repo", "evil.example", []string{"org", "repo"}},
		{"ssh with user and port", "ssh://git@host.example:2222/org/repo", "host.example", []string{"org", "repo"}},
		{"git+ssh", "git+ssh://git@host.example/org/repo.git", "host.example", []string{"org", "repo"}},
		{"scp-like", "git@github.com:org/repo.git", "github.com", []string{"org", "repo"}},
		{"scp-like without a user part", "deploy@host.example:org/repo", "host.example", []string{"org", "repo"}},
		{"scp-like uses the last at sign", "a@evil.example@github.com:org/repo", "github.com", []string{"org", "repo"}},
		{"ipv6 literal", "https://[::1]/org/repo", "::1", []string{"org", "repo"}},
		{"ipv6 literal with a port", "https://[::1]:8443/org/repo", "::1", []string{"org", "repo"}},
		{"ipv6 is canonicalized", "https://[0:0:0::1]/org/repo", "::1", []string{"org", "repo"}},
		{"ipv6 scp-like", "git@[::1]:org/repo.git", "::1", []string{"org", "repo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			host, segs, ok, err := splitLocation(tt.source)
			// Assert
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, tt.wantHost, host)
			assert.Equal(t, tt.wantSegs, segs)
		})
	}
}

func TestSplitLocationLocalSourcesNameNoHost(t *testing.T) {
	for _, src := range []string{"", "./vendor/rules", "/abs/path", "file:///tmp/repo", "git+file:///tmp/repo", "C:\\x"} {
		t.Run(src, func(t *testing.T) {
			_, _, ok, err := splitLocation(src)
			require.NoError(t, err)
			assert.False(t, ok)
		})
	}
}

func TestSplitLocationFailsClosed(t *testing.T) {
	for _, src := range []string{
		"https://evil.example\\@github.com/org/repo",
		"https://github.com/org\\repo",
		"https://evil.example /org/repo",
		"https://evil.example\t/org/repo",
		"https://github.com%2eevil.example/org/repo",
		"https://[::1/org/repo",
		"https://:443/org/repo",
		"https:///org/repo",
		"https://host_é.example/org/repo",
		"git@host.example?x:org/repo",
		"git@:org/repo",
	} {
		t.Run(src, func(t *testing.T) {
			_, _, ok, err := splitLocation(src)
			assert.Error(t, err)
			assert.False(t, ok)
		})
	}
}

func TestSourceCheckerHostOutcomes(t *testing.T) {
	tests := []struct {
		name   string
		allow  []string
		deny   []string
		source string
		want   bool
	}{
		{"allowed host", []string{"github.com"}, nil, "https://github.com/org/repo.git", true},
		{"query bypass is refused", []string{"github.com"}, nil, "https://evil.example?@github.com/org/repo.git", false},
		{"fragment bypass is refused", []string{"github.com"}, nil, "https://evil.example#@github.com/org/repo.git", false},
		{"backslash bypass is refused", []string{"github.com"}, nil, "https://evil.example\\@github.com/org/repo.git", false},
		{"userinfo names no allowed host", []string{"github.com"}, nil, "https://github.com@evil.example/org/repo.git", false},
		{"denied host with a query trick", nil, []string{"evil.example"}, "https://evil.example?@github.com/o/r", false},
		{"unparseable is refused even when only deny is set", nil, []string{"evil.example"}, "https://a b/o/r", false},
		{"uppercase and trailing dot", []string{"github.com"}, nil, "https://GITHUB.com./org/repo", true},
		{"denied with a port", nil, []string{"github.com"}, "https://github.com:8443/org/repo", false},
		{"ipv6 denied", nil, []string{"[::1]"}, "https://[::1]:8443/org/repo", false},
		{"ipv6 allowed", []string{"[::1]"}, nil, "https://[::1]/org/repo", true},
		{"ipv6 not allowed", []string{"github.com"}, nil, "https://[::1]/org/repo", false},
		{"scp-like allowed", []string{"github.com/org"}, nil, "git@github.com:org/repo.git", true},
		{"scp-like denied", nil, []string{"github.com/org"}, "git@github.com:org/repo.git", false},
		{"marketplace-named plugin repo path", []string{"github.com/org"}, nil, "https://github.com/other/plugin-repo.git", false},
		{"marketplace-named plugin repo allowed", []string{"github.com/org"}, nil, "https://github.com/org/plugin-repo.git", true},
		{"local path is not governed", []string{"github.com"}, nil, "./vendor/x", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			src := Sources{Deny: tt.deny}
			if tt.allow != nil {
				src.Allowed = List{Set: true, Items: tt.allow}
			}
			res := Resolve([]Layer{layer("managed", Policy{Sources: src})})
			a := &applier{res: res, cfg: &config.Config{}, out: &config.PolicyOutcome{}}
			check := a.sourceChecker()
			require.NotNil(t, check)
			// Act
			got := check("include", "x", tt.source)
			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIPv6Patterns(t *testing.T) {
	p, err := parsePattern("[::1]/org")
	require.NoError(t, err)
	assert.Equal(t, "::1", p.host)
	assert.Equal(t, "[::1]/org", p.String())
	for _, bad := range []string{"[::1", "[::1]x", "[nothex]", "[::1]:8443"} {
		_, err := parsePattern(bad)
		assert.Error(t, err, bad)
	}
}
