package includes

import "regexp"

// userinfoRe matches the credentials in a URL's authority: scheme://user:pass@.
// scp-style remotes (git@host:owner/repo) have no scheme and are left alone.
var userinfoRe = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^/\s@]+@`)

// redactURL hides URL userinfo (a user name, user:password or a token) so a
// repository address can be logged or put into an error context without leaking
// the credential embedded in it. It also cleans free text such as git output,
// which echoes the URL it was given.
func redactURL(s string) string {
	return userinfoRe.ReplaceAllString(s, "${1}<redacted>@")
}
