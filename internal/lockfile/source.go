package lockfile

import (
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

// gitURLSchemes are the URL schemes git can fetch from. Anything else with a
// scheme-like prefix is a local path.
var gitURLSchemes = []string{"http://", "https://", "file://", "ssh://", "git://", "git+ssh://", "git+https://", "git+http://"}

// IsGitSource reports whether an include or skill source is a git repository
// rather than a local path.
//
// Git sources are URLs with a git-capable scheme (http, https, file, ssh, git
// and the git+ forms) and scp-like addresses, "[user@]host:path" with an "@"
// before the first ":" and no "/" before it ("git@github.com:org/repo.git").
// Everything else, including Windows drive paths (C:\x, C:/x), is local.
func IsGitSource(source string) bool {
	lower := strings.ToLower(source)
	for _, scheme := range gitURLSchemes {
		if strings.HasPrefix(lower, scheme) {
			return true
		}
	}
	colon := strings.Index(source, ":")
	at := strings.Index(source, "@")
	return colon > 0 && at > 0 && at < colon && !strings.ContainsAny(source[:colon], `/\`)
}

// githubShorthand is the GitHub "owner/repo" form.
var githubShorthand = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9._-]+$`)

// IsGitHubShorthand reports whether s is a GitHub "owner/repo" shorthand.
func IsGitHubShorthand(s string) bool { return githubShorthand.MatchString(s) }

// GitHubURL is the https clone URL of an "owner/repo" shorthand.
func GitHubURL(shorthand string) string { return "https://github.com/" + shorthand }

// FileURLPath returns the filesystem path a file:// (or git+file://) source
// names, and false for every other source.
func FileURLPath(source string) (string, bool) {
	s := source
	if len(s) >= 4 && strings.EqualFold(s[:4], "git+") {
		s = s[4:]
	}
	if len(s) < len("file://") || !strings.EqualFold(s[:len("file://")], "file://") {
		return "", false
	}
	u, err := url.Parse(s)
	if err != nil || u.Path == "" {
		return strings.TrimPrefix(s[len("file://"):], "/"), true
	}
	path := u.Path
	switch {
	case len(u.Host) == 2 && u.Host[1] == ':' && isASCIILetter(u.Host[0]):
		// With two slashes the URL parser takes the drive letter for the host.
		path = u.Host + path
	case runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' && isASCIILetter(path[1]):
		// With three slashes the path keeps a leading slash before the drive letter.
		path = path[1:]
	}
	return filepath.FromSlash(path), true
}

func isASCIILetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }

// EnvAllowFileURLs, when set to "1" in the environment of the user running
// ai-rulez, lets a file:// git source outside the project resolve from the
// project config. A repository cannot set it; the machine-local overlay and the
// user config need no such opt-in.
const EnvAllowFileURLs = "AI_RULEZ_ALLOW_FILE_URLS"

// AllowFileURLsOutside reports whether the user opted in to file:// sources that
// leave the project (EnvAllowFileURLs).
func AllowFileURLsOutside(env ambient.Env) bool { return ambient.Getenv(env, EnvAllowFileURLs) == "1" }
