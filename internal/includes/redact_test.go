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
		{"password containing an at sign", "https://user:p@ssw0rd@127.0.0.1:1/o/r.git", "https://<redacted>@127.0.0.1:1/o/r.git"},
		{"at sign in the query is not userinfo", "https://example.com?mail=a@b", "https://example.com?mail=<redacted>"},
		{"http scheme", "http://u:p@host/x", "http://<redacted>@host/x"},
		{"ssh scheme", "ssh://git@github.com/o/r.git", "ssh://<redacted>@github.com/o/r.git"},
		{"no userinfo", "https://github.com/o/r.git", "https://github.com/o/r.git"},
		{"scp-style stays unchanged", "git@github.com:o/r.git", "git@github.com:o/r.git"},
		{"file url", "file:///tmp/repo", "file:///tmp/repo"},
		{"at sign only in the path", "https://github.com/o/r@v1", "https://github.com/o/r@v1"},
		{"git output echoing the url", "fatal: unable to access 'https://tok:x-oauth-basic@github.com/o/r/': 403", "fatal: unable to access 'https://<redacted>@github.com/o/r/': 403"},
		{"query token", "https://example.com/r.git?access_token=SECRET", "https://example.com/r.git?access_token=<redacted>"},
		{"several params", "https://example.com/r?a=1&token=SECRET&flag", "https://example.com/r?a=<redacted>&token=<redacted>&flag"},
		{"query before fragment", "https://example.com/r?k=SECRET#frag", "https://example.com/r?k=<redacted>#frag"},
		{"userinfo and query", "https://u:p@example.com/r?k=SECRET", "https://<redacted>@example.com/r?k=<redacted>"},
		{"quoted url in git output", "fatal: unable to access 'https://example.com/r?k=SECRET': 403", "fatal: unable to access 'https://example.com/r?k=<redacted>': 403"},
		{"question mark without scheme", "./dir?name=x", "./dir?name=x"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RedactURL(tt.in))
		})
	}
}

func TestValidateGitURL_ErrorDoesNotLeakUserinfo(t *testing.T) {
	err := validateGitURL("ftp://user:pw@host/repo")

	if assert.Error(t, err) {
		assert.NotContains(t, err.Error(), "pw@")
	}
}
