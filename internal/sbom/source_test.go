package sbom

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestSourceComponent(t *testing.T) {
	tests := []struct {
		name, source, ref, path, wantPURL, wantVCS string
	}{
		{"github https", "https://github.com/Org/Rules.git", "v1", "", "pkg:github/org/rules@v1", "https://github.com/Org/Rules"},
		{"github scp", "git@github.com:Org/Rules.git", "", "sub/dir", "pkg:github/org/rules#sub/dir", "https://github.com/Org/Rules"},
		{"gitlab ssh", "ssh://git@gitlab.com/grp/proj.git", "main", "", "pkg:gitlab/grp/proj@main", "https://gitlab.com/grp/proj"},
		{"bitbucket", "https://bitbucket.org/team/repo", "", "", "pkg:bitbucket/team/repo", "https://bitbucket.org/team/repo"},
		{"other host", "https://git.example.com/team/rules", "v2", "", "pkg:generic/shared@v2?vcs_url=git%2Bhttps://git.example.com/team/rules", "https://git.example.com/team/rules"},
		{"credentials and query dropped", "https://user:ghp_tok3n@github.com/Org/Rules.git?access_token=q", "v1", "", "pkg:github/org/rules@v1", "https://github.com/Org/Rules"},
		{"local file", "file:///srv/rules", "", "", "", ""},
		{"local path", "../shared", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			comp := sourceComponent(lockfile.KindInclude, "shared", tt.source, tt.ref, tt.path, nil)

			// Assert
			assert.Equal(t, tt.wantPURL, comp.PURL)
			encoded, err := json.Marshal(comp)
			require.NoError(t, err)
			for _, leak := range []string{"ghp_tok3n", "access_token", "user:", "/srv/rules", "../shared"} {
				assert.NotContains(t, string(encoded), leak)
			}
			if tt.wantVCS == "" {
				assert.Empty(t, comp.ExternalReferences)
				return
			}
			require.Len(t, comp.ExternalReferences, 1)
			assert.Equal(t, tt.wantVCS, comp.ExternalReferences[0].URL)
		})
	}
}

func TestSourceComponentUsesTheLockedCommit(t *testing.T) {
	// Arrange
	pin := &lockfile.Entry{Name: "shared", Commit: "0123456789abcdef", Digest: "sha256:abc"}

	// Act
	comp := sourceComponent(lockfile.KindInclude, "shared", "https://github.com/Org/Rules", "v1", "", pin)

	// Assert
	assert.Equal(t, "0123456789abcdef", comp.Version)
	assert.Equal(t, "pkg:github/org/rules@0123456789abcdef", comp.PURL)
	assert.Contains(t, comp.Properties, Property{Name: "ai-rulez:digest", Value: "sha256:abc"})
	assert.Contains(t, comp.Properties, Property{Name: "ai-rulez:ref", Value: "v1"})
	assert.Contains(t, comp.Properties, Property{Name: "ai-rulez:commit", Value: "0123456789abcdef"})
}

func TestRedactEndpoint(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"https://u:p@example.com/mcp?key=v#f", "https://example.com/mcp", true},
		{"http://localhost:8080/sse", "http://localhost:8080/sse", true},
		{"${SECRET_URL}", "", false},
		{"https://${HOST}/mcp", "", false},
	}
	for _, tt := range tests {
		got, ok := redactEndpoint(tt.in)
		assert.Equal(t, tt.ok, ok, tt.in)
		assert.Equal(t, tt.want, got, tt.in)
	}
}
