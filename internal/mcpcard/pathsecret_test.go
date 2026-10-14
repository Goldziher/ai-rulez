package mcpcard

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuild_URLPathThatLooksLikeACredentialIsSkippedWithANote(t *testing.T) {
	tests := []struct {
		name string
		url  string
		leak string
	}{
		{"short sk- key", "https://a.test/mcp/sk-abc123", "sk-abc123"},
		{"github token", "https://a.test/mcp/ghp_" + strings.Repeat("a1", 18), "ghp_"},
		{"percent-encoded prefix", "https://a.test/mcp/%73k-abc12345", "abc12345"},
		{"long opaque segment", "https://a.test/hooks/" + strings.Repeat("x9", 20) + "/mcp", strings.Repeat("x9", 20)},
		{"aws key id", "https://a.test/AKIAABCDEFGHIJKLMNOP/mcp", "AKIAABCDEFGHIJKLMNOP"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			servers := []Server{remote("leaky", tt.url), remote("docs", "https://docs.test/mcp")}

			// Act
			cards, notes, err := Build(servers, Options{Namespace: "test.acme", Version: "1.0.0"})

			// Assert
			require.NoError(t, err)
			require.Len(t, cards, 1)
			assert.Equal(t, "test.acme/docs", cards[0].Name)
			require.Len(t, notes, 1)
			assert.Contains(t, notes[0], `"leaky"`)
			assert.Contains(t, notes[0], "path")
			assert.NotContains(t, notes[0], tt.leak)
			assert.NotContains(t, notes[0], "a.test", "the URL is never printed")
		})
	}
}

func TestBuild_OrdinaryURLPathsArePublishedAsWritten(t *testing.T) {
	for _, url := range []string{
		"https://a.test/mcp",
		"https://a.test/v1/servers/docs/mcp",
		"https://a.test/tenants/acme-corp/mcp",
		"https://a.test/task-skills/mcp",
		"https://a.test/sk/mcp",
	} {
		t.Run(url, func(t *testing.T) {
			// Act
			cards, notes, err := Build([]Server{remote("docs", url)}, Options{Namespace: "test.acme", Version: "1.0.0"})

			// Assert
			require.NoError(t, err)
			require.Len(t, cards, 1)
			assert.Empty(t, notes)
			assert.Equal(t, url, cards[0].Remotes[0].URL)
		})
	}
}
