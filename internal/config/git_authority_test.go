package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGitSourceAuthorityProblems(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"query trick", "https://evil.example?@github.com/org/repo.git", "'?' and '#'"},
		{"fragment trick", "https://evil.example#@github.com/org/repo.git", "'?' and '#'"},
		{"backslash trick", "https://evil.example\\@github.com/org/repo.git", "backslash"},
		{"space in the host", "https://evil.example /org/repo.git", "whitespace"},
		{"user in https", "https://github.com@evil.example/org/repo.git", "credentials"},
		{"user in git://", "git://u@host.example/org/repo.git", "credentials"},
		{"two users in ssh", "ssh://a@b@host.example/org/repo.git", "credentials"},
		{"scp-like query", "git@evil.example?x:org/repo.git", "invalid host"},
		{"scp-like two at signs", "a@b@host.example:org/repo.git", "invalid host"},
		{"plain https", "https://github.com/org/repo.git", ""},
		{"https with a port and query on the path", "https://github.com:8443/org/repo.git?ref=x", ""},
		{"ssh keeps its user", "ssh://git@host.example:2222/org/repo.git", ""},
		{"scp-like", "git@github.com:org/repo.git", ""},
		{"ipv6", "https://[::1]:8443/org/repo.git", ""},
		{"file url", "file:///tmp/repo", ""},
		{"local path", "./vendor/skills", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			skill := skillSourceProblem(tt.source)
			plugin := pluginSourceProblem(tt.source)
			// Assert
			if tt.want == "" {
				assert.Empty(t, skill)
				assert.Empty(t, plugin)
				return
			}
			assert.Contains(t, skill, tt.want)
			assert.Contains(t, plugin, tt.want)
		})
	}
}
