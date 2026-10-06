package sbom

import (
	"net/url"
	"strings"
)

// gitLocation is where a git source lives, without credentials.
type gitLocation struct {
	Scheme string // https, http, ssh, git
	Host   string // host[:port]
	Path   string // owner/repo, no leading slash, no .git
}

// HTTPS is the browsable URL of the repository; it never carries userinfo, a
// query or a fragment.
func (g gitLocation) HTTPS() string {
	scheme := "https"
	if g.Scheme == "http" {
		scheme = "http"
	}
	return scheme + "://" + g.Host + "/" + g.Path
}

// parseGitSource splits a git source into host and path. ok is false for a
// local path or a file:// URL, which are never put into the document.
func parseGitSource(source string) (loc gitLocation, ok bool) {
	source = strings.TrimSpace(source)
	if i := strings.Index(source, "://"); i > 0 {
		scheme := strings.ToLower(source[:i])
		scheme = strings.TrimPrefix(scheme, "git+")
		if scheme == "file" {
			return gitLocation{}, false
		}
		u, err := url.Parse(source)
		if err != nil || u.Hostname() == "" {
			return gitLocation{}, false
		}
		host := u.Hostname()
		if port := u.Port(); port != "" && scheme != "ssh" {
			host += ":" + port
		}
		return gitLocation{Scheme: scheme, Host: host, Path: cleanRepoPath(u.Path)}, true
	}
	// scp-like: [user@]host:path
	colon, at := strings.Index(source, ":"), strings.Index(source, "@")
	if colon > 0 && at > 0 && at < colon && !strings.ContainsAny(source[:colon], `/\`) {
		return gitLocation{Scheme: "ssh", Host: source[at+1 : colon], Path: cleanRepoPath(source[colon+1:])}, true
	}
	return gitLocation{}, false
}

func cleanRepoPath(p string) string {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	p = strings.Trim(p, "/")
	return strings.TrimSuffix(p, ".git")
}

// redactEndpoint returns an MCP server URL without userinfo, query or fragment.
// ok is false when the URL cannot be parsed (for example it is a ${VAR}
// placeholder), in which case nothing is emitted.
func redactEndpoint(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}
	u.User, u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = nil, "", false, "", ""
	return u.String(), true
}
