package publish

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

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
	git := gitutil.New(r)
	out := func(args ...string) (string, bool) {
		res := git.Exec(ctx, dir, nil, args...)
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
	args := []string{"status", "--porcelain", "--", "."}
	if excludeRel != "" && excludeRel != "." {
		args = append(args, ":(exclude,top)"+excludeRel)
	}
	if status, ok := out(args...); ok {
		info.Source.Dirty = status != ""
	}
	if remote, ok := out("remote", "get-url", "origin"); ok {
		info.Remote = StripCredentials(remote)
	}
	return info
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
