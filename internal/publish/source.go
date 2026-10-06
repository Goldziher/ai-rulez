package publish

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// gitQueryTimeout bounds each of the cheap local git queries ReadSource makes;
// a hung git must not stall publish for the runner's 15 minute maximum.
const gitQueryTimeout = 30 * time.Second

// SourceInfo is what git says about the project being published.
type SourceInfo struct {
	Source Source
	// Mtime is the committer time of HEAD, or 0 without a commit.
	Mtime int64
	// Remote is the origin URL with credentials removed, or "".
	Remote string
}

// ReadSource asks git about dir. A directory outside a repository, or a
// repository without a commit, reports Dirty = true: a tree that cannot be tied
// to a commit cannot be called clean. excludeRel is a slash path (the dist
// directory) whose changes are ignored when it lies inside the repository.
func ReadSource(ctx context.Context, r runner.Runner, dir, excludeRel string) SourceInfo {
	out := func(args ...string) (string, bool) {
		argv := append([]string{"git"}, args...)
		if dir != "" {
			argv = append([]string{"git", "-C", dir}, args...)
		}
		res := runner.Or(r).Run(ctx, runner.Spec{
			Argv:    argv,
			Env:     gitutil.Env(nil),
			Timeout: gitQueryTimeout,
		})
		return strings.TrimSpace(string(res.Stdout)), res.Status == runner.StatusOK
	}
	info := SourceInfo{Source: Source{Dirty: true}}
	commit, ok := out("rev-parse", "--verify", "HEAD")
	if !ok {
		return info
	}
	info.Source.Commit = commit
	if ct, ok := out("log", "-1", "--format=%ct"); ok {
		if n, err := strconv.ParseInt(ct, 10, 64); err == nil && n > 0 {
			info.Mtime = n
		}
	}
	args := []string{"status", "--porcelain", "--untracked-files=all", "--", "."}
	if excludeRel != "" && excludeRel != "." {
		args = append(args, ":(exclude,top)"+excludeRel)
	}
	if status, ok := out(args...); ok {
		info.Source.Dirty = status != ""
	}
	if remote, ok := out("remote", "get-url", "origin"); ok {
		info.Remote = PublicRemote(remote)
	}
	return info
}

// PublicRemote is StripCredentials for a remote that may be published: a local
// path or a file:// URL (a clone from disk) is dropped, because the manifest
// records the repository and must not carry the publisher's directory layout.
func PublicRemote(remote string) string {
	r := strings.TrimSpace(remote)
	switch {
	case strings.HasPrefix(strings.ToLower(r), "file:"),
		strings.HasPrefix(r, "/"), strings.HasPrefix(r, "./"), strings.HasPrefix(r, "../"), strings.HasPrefix(r, "~"),
		strings.HasPrefix(r, `\`), r == "." || r == "..",
		len(r) >= 3 && r[1] == ':' && (r[2] == '\\' || r[2] == '/'):
		return ""
	}
	return StripCredentials(r)
}

// StripCredentials removes userinfo and a query from a URL; scp-style and local
// remotes are returned unchanged apart from a leading "user@" being kept (it
// carries no secret).
func StripCredentials(remote string) string {
	if !strings.Contains(remote, "://") {
		return remote
	}
	u, err := url.Parse(remote)
	if err != nil {
		return ""
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	return u.String()
}

// PreviousTag returns the closest tag reachable from HEAD other than exclude,
// the usual "previous release" for the release notes; "" when there is none or
// git cannot say. The tag text is validated before it can reach an argv.
func PreviousTag(ctx context.Context, r runner.Runner, dir, exclude string) string {
	return PreviousTagMatching(ctx, r, dir, exclude, "")
}

// PreviousTagMatching is PreviousTag limited to tags that match the glob (such
// as "acme-v*", the tags of one plugin of a multi-plugin release). A glob
// outside the tag alphabet is ignored.
func PreviousTagMatching(ctx context.Context, r runner.Runner, dir, exclude, glob string) string {
	argv := []string{"git"}
	if dir != "" {
		argv = append(argv, "-C", dir)
	}
	argv = append(argv, "describe", "--tags", "--abbrev=0")
	if glob != "" && tagPattern.MatchString(strings.TrimSuffix(glob, "*")) {
		argv = append(argv, "--match", glob)
	}
	if exclude != "" && tagPattern.MatchString(exclude) {
		argv = append(argv, "--exclude", exclude)
	}
	argv = append(argv, "HEAD")
	res := runner.Or(r).Run(ctx, runner.Spec{Argv: argv, Env: gitutil.Env(nil), Timeout: gitQueryTimeout})
	tag := strings.TrimSpace(string(res.Stdout))
	if res.Status != runner.StatusOK || !tagPattern.MatchString(tag) || strings.Contains(tag, "..") {
		return ""
	}
	return tag
}

// RepoFromURL turns a GitHub-style remote into OWNER/REPO (or HOST/OWNER/REPO
// for a host other than github.com). It returns "" when remote has no such shape.
func RepoFromURL(remote string) string {
	s := strings.TrimSpace(remote)
	var host, rest string
	switch {
	case strings.Contains(s, "://"):
		u, err := url.Parse(s)
		if err != nil {
			return ""
		}
		host, rest = u.Hostname(), strings.TrimPrefix(u.Path, "/")
	case strings.Contains(s, "@") && strings.Contains(s, ":"):
		after := s[strings.Index(s, "@")+1:]
		host, rest, _ = strings.Cut(after, ":")
	default:
		return strings.TrimSuffix(s, ".git")
	}
	rest = strings.TrimSuffix(strings.TrimSuffix(rest, "/"), ".git")
	if host == "" || strings.Count(rest, "/") != 1 {
		return ""
	}
	if host == "github.com" {
		return rest
	}
	return host + "/" + rest
}
