package importer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRemotes_FetchesOnlyHTTPS(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantFetch bool
	}{
		{"https", "https://github.com/acme/skills", true},
		{"https upper case", "HTTPS://github.com/acme/skills", true},
		{"ssh", "ssh://git@github.com/acme/skills", false},
		{"scp", "git@github.com:acme/skills.git", false},
		{"file", "file:///etc", false},
		{"local path", "/srv/repo", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			fake := &fakeFetcher{err: assert.AnError}
			p := &Plan{Remotes: []Remote{{Kind: remoteRules, Origin: "rulesync.jsonc#sources.x", URL: tt.url}}}

			// Act
			err := p.resolveRemotes(context.Background(), Options{Fetch: true, Fetcher: fake})

			// Assert
			if tt.wantFetch {
				require.Error(t, err, "the fake fails every fetch it is asked for")
				assert.Len(t, fake.calls, 1)
				return
			}
			require.NoError(t, err)
			assert.Empty(t, fake.calls, "a non-https source never reaches the fetcher")
			f := findingFor(p, StatusNeedsAction, "rulesync.jsonc", "sources.x")
			require.NotNil(t, f)
			assert.Contains(t, f.Reason, "https")
		})
	}
}
