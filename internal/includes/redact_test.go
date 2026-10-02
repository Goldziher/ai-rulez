package includes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"token as user", "https://ghp_abc123@github.com/o/r.git", "https://<redacted>@github.com/o/r.git"},
		{"user and password", "https://user:pw@example.com/o/r", "https://<redacted>@example.com/o/r"},
		{"injected token form", "https://tok:x-oauth-basic@github.com/o/r", "https://<redacted>@github.com/o/r"},
		{"http scheme", "http://u:p@host/x", "http://<redacted>@host/x"},
		{"ssh scheme", "ssh://git@github.com/o/r.git", "ssh://<redacted>@github.com/o/r.git"},
		{"no userinfo", "https://github.com/o/r.git", "https://github.com/o/r.git"},
		{"scp-style stays unchanged", "git@github.com:o/r.git", "git@github.com:o/r.git"},
		{"file url", "file:///tmp/repo", "file:///tmp/repo"},
		{"at sign only in the path", "https://github.com/o/r@v1", "https://github.com/o/r@v1"},
		{"git output echoing the url", "fatal: unable to access 'https://tok:x-oauth-basic@github.com/o/r/': 403", "fatal: unable to access 'https://<redacted>@github.com/o/r/': 403"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, redactURL(tt.in))
		})
	}
}

func TestInjectTokenIsRedactedForDisplay(t *testing.T) {
	// The credential-bearing form git receives must never survive redaction.
	injected := injectToken("https://github.com/o/r.git", "ghp_secret")

	assert.Contains(t, injected, "ghp_secret")
	assert.NotContains(t, redactURL(injected), "ghp_secret")
}

func TestValidateGitURL_ErrorDoesNotLeakUserinfo(t *testing.T) {
	err := validateGitURL("ftp://user:pw@host/repo")

	if assert.Error(t, err) {
		assert.NotContains(t, err.Error(), "pw@")
	}
}
