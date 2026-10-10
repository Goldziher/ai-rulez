// Package urlredact hides the credentials in a URL and in free text that echoes
// one, so an address can be logged, returned to a caller or put into an error
// context without leaking the secret embedded in it. It has no dependencies
// beyond the standard library so that low-level packages (gitutil) and the
// include loader (includes) can share one implementation.
package urlredact

import (
	"net/url"
	"regexp"
	"strings"
)

// credentialParamName reports whether a query-string parameter name commonly
// carries a token, so a URL with it is treated as credentialed even without
// userinfo.
func credentialParamName(name string) bool {
	switch strings.ToLower(name) {
	case "access_token", "access-token", "token", "private_token", "private-token",
		"auth", "authorization", "api_key", "api-key", "apikey", "key",
		"password", "passwd", "pwd", "secret", "client_secret", "client-secret",
		"signature", "sig", "bearer", "credential", "credentials":
		return true
	}
	return false
}

// HasCredentials reports whether a git URL carries a credential that would be
// written into config.toml as a readable secret: userinfo (a user name,
// user:password pair or token) in its authority, or a credential-looking query
// parameter such as ?access_token=... . Only http(s) URLs are checked: an ssh://
// remote's user (git@host) is the login name, not a credential, and a scp-style
// git@host:owner/repo has no scheme at all. A git+https:// prefix is accepted.
func HasCredentials(raw string) bool {
	u, err := url.Parse(strings.TrimPrefix(strings.TrimSpace(raw), "git+"))
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return false
	}
	if u.User != nil {
		return true
	}
	for name := range u.Query() {
		if credentialParamName(name) {
			return true
		}
	}
	return false
}

// userinfoRe matches the credentials in a URL's authority: scheme://user:pass@.
// The password may itself contain "@", so the match runs to the last "@" before
// the first "/", "?" or "#". scp-style remotes (git@host:owner/repo) have no
// scheme and are left alone.
var userinfoRe = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^/\s?#]+@`)

// queryRe matches the query string of a scheme://... URL (up to the fragment or
// whitespace), where tokens such as ?access_token=... are commonly passed.
var queryRe = regexp.MustCompile(`(?i)(\b[a-z][a-z0-9+.-]*://[^\s?#]*\?)([^\s#'"]*)`)

const redactedValue = "<redacted>"

// HasUserinfo reports whether s contains a URL with credentials in its authority
// (scheme://user@host) anywhere in the text. It is used to spot a credential a
// previous version left in a cache's .git/config.
func HasUserinfo(s string) bool { return userinfoRe.MatchString(s) }

// URL hides URL userinfo (a user name, user:password or a token) and the values
// of query-string parameters, so a repository address can be logged, returned to
// a caller or put into an error context without leaking the credential embedded
// in it. The rest of the URL, including parameter names, is kept. It also cleans
// free text such as git output, which echoes the URL it was given.
func URL(s string) string {
	s = userinfoRe.ReplaceAllString(s, "${1}"+redactedValue+"@")
	return queryRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := queryRe.FindStringSubmatch(m)
		params := strings.Split(parts[2], "&")
		for i, p := range params {
			if name, _, ok := strings.Cut(p, "="); ok {
				params[i] = name + "=" + redactedValue
			}
		}
		return parts[1] + strings.Join(params, "&")
	})
}
