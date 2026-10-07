package includes

import (
	"regexp"
	"strings"
)

// userinfoRe matches the credentials in a URL's authority: scheme://user:pass@.
// The password may itself contain "@", so the match runs to the last "@" before
// the first "/", "?" or "#". scp-style remotes (git@host:owner/repo) have no scheme and are left alone.
var userinfoRe = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^/\s?#]+@`)

// queryRe matches the query string of a scheme://... URL (up to the fragment or
// whitespace), where tokens such as ?access_token=... are commonly passed.
var queryRe = regexp.MustCompile(`(?i)(\b[a-z][a-z0-9+.-]*://[^\s?#]*\?)([^\s#'"]*)`)

const redactedValue = "<redacted>"

// RedactURL hides URL userinfo (a user name, user:password or a token) and the
// values of query-string parameters, so a repository address can be logged,
// returned to a caller or put into an error context without leaking the
// credential embedded in it. The rest of the URL, including parameter names, is
// kept. It also cleans free text such as git output, which echoes the URL it was
// given.
func RedactURL(s string) string {
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
