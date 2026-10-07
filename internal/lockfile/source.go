package lockfile

import (
	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"net/url"
	"path/filepath"
	"strings"
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
	return filepath.FromSlash(u.Path), true
}

// EnvAllowFileURLs, when set to "1" in the environment of the user running
// ai-rulez, lets a file:// git source outside the project resolve from the
// project config. A repository cannot set it; the machine-local overlay and the
// user config need no such opt-in.
const EnvAllowFileURLs = "AI_RULEZ_ALLOW_FILE_URLS"

// AllowFileURLsOutside reports whether the user opted in to file:// sources that
// leave the project (EnvAllowFileURLs).
func AllowFileURLsOutside(env ambient.Env) bool { return ambient.Getenv(env, EnvAllowFileURLs) == "1" }
