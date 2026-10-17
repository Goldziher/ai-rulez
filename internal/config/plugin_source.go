package config

import (
	"net/url"
	"regexp"
	"strings"
	"sync"
)

// Source forms of a marketplace or plugin entry.
const (
	// PluginSourceGit is a git URL (https, ssh, git@host:path, file://).
	PluginSourceGit = "git"
	// PluginSourceGitHub is the owner/repo shorthand (or github:owner/repo).
	PluginSourceGitHub = "github"
	// PluginSourceLocal is a directory on disk.
	PluginSourceLocal = "local"
	// PluginSourceIndexURL is an https URL of a marketplace.json file.
	PluginSourceIndexURL = "index-url"
)

// schemeHTTPS is the URL scheme remote sources and endpoints must use.
const schemeHTTPS = "https"

const githubBase = "https://github.com/"

var githubShorthand = sync.OnceValue(func() *regexp.Regexp {
	return regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
})

// ClassifyPluginSource says which form a marketplace or plugin source is. A
// local path starts with ./, ../, / or ~ (or is absolute on Windows); a bare
// owner/repo is a GitHub repository, as in Claude Code.
func ClassifyPluginSource(source string) string {
	src := strings.TrimSpace(source)
	lower := strings.ToLower(src)
	switch {
	case strings.HasPrefix(lower, "git+"):
		return ClassifyPluginSource(src[4:])
	case strings.HasPrefix(lower, "github:"):
		return PluginSourceGitHub
	case strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://"):
		if isIndexURL(src) {
			return PluginSourceIndexURL
		}
		return PluginSourceGit
	case hasGitScheme(src, lower):
		return PluginSourceGit
	case hasLocalPrefix(src):
		return PluginSourceLocal
	case githubShorthand().MatchString(src):
		return PluginSourceGitHub
	}
	return PluginSourceLocal
}

func hasGitScheme(src, lower string) bool {
	for _, p := range []string{"ssh://", "file://", "git://"} {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return strings.HasPrefix(src, "git@")
}

func hasLocalPrefix(src string) bool {
	for _, p := range []string{"./", "../", "/", "~", "\\"} {
		if strings.HasPrefix(src, p) {
			return true
		}
	}
	return src == "." || src == ".." || (len(src) >= 2 && src[1] == ':')
}

func isIndexURL(src string) bool {
	u, err := url.Parse(src)
	return err == nil && strings.HasSuffix(strings.ToLower(u.Path), ".json")
}

// PluginGitURL turns a git or github-shorthand source into the URL git clones.
func PluginGitURL(source string) string {
	src := strings.TrimSpace(source)
	if len(src) >= 4 && strings.EqualFold(src[:4], "git+") {
		src = src[4:]
	}
	if len(src) >= 7 && strings.EqualFold(src[:7], "github:") {
		src = src[7:]
		return githubBase + strings.TrimSuffix(src, ".git") + ".git"
	}
	if ClassifyPluginSource(src) == PluginSourceGitHub {
		return githubBase + strings.TrimSuffix(src, ".git") + ".git"
	}
	return src
}

// pluginSourceProblem says why a marketplace or plugin source is refused ("" when it is fine):
// the git safety rules of installed skills, plus no credentials in the URL and https for an index.
func pluginSourceProblem(source string) string {
	if reason := skillSourceProblem(source); reason != "" {
		return reason
	}
	src := strings.TrimSpace(source)
	switch ClassifyPluginSource(src) {
	case PluginSourceIndexURL:
		u, err := url.Parse(src)
		if err != nil || u.Scheme != schemeHTTPS || u.Host == "" {
			return "a marketplace index URL must be https"
		}
		if u.User != nil {
			return "must not embed credentials in the URL"
		}
	case PluginSourceGit:
		return gitCredentialProblem(src)
	case PluginSourceGitHub:
		if strings.Contains(src, "..") {
			return "is not a valid owner/repo"
		}
	case PluginSourceLocal:
	}
	return ""
}

// gitCredentialProblem refuses a user (a password or a token) embedded in a web URL; ssh keeps its user.
func gitCredentialProblem(src string) string {
	u, err := url.Parse(strings.TrimPrefix(src, "git+"))
	if err != nil || u.User == nil || u.Scheme == "ssh" {
		return ""
	}
	if _, hasPassword := u.User.Password(); hasPassword || u.Scheme == schemeHTTPS || u.Scheme == "http" {
		return "must not embed credentials in the URL (use the git credential helper or AI_RULEZ_GIT_TOKEN)"
	}
	return ""
}

// PluginSourceProblem says why a marketplace or plugin source is refused ("" when it is fine).
func PluginSourceProblem(source string) string { return pluginSourceProblem(source) }

// PluginRefProblem says why a ref cannot be handed to the fetcher ("" when it can).
func PluginRefProblem(ref string) string { return skillRefProblem(ref) }

// PluginPathProblem says why a path cannot address a directory inside a repository ("" when it can).
func PluginPathProblem(p string) string { return skillPathProblem(p) }
