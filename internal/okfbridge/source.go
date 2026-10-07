package okfbridge

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

const quiet = "--quiet"

const cloneTimeout = 3 * time.Minute

// Source is a bundle location: a directory or a git repository.
type Source struct {
	// Dir is a local directory. Empty for git sources.
	Dir string
	// URL, Ref and Subdir describe a git source; Subdir is the bundle directory inside the repository.
	URL, Ref, Subdir string
}

var scpLike = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+:`)

// ParseSource understands `path`, `https://host/org/repo[.git][@ref][#subdir]`,
// `git@host:org/repo[.git][@ref][#subdir]` and `file:///path/repo[@ref][#subdir]`.
func ParseSource(spec string) (Source, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return Source{}, fmt.Errorf("no bundle source given")
	}
	isGit := strings.HasPrefix(spec, "https://") || strings.HasPrefix(spec, "ssh://") ||
		strings.HasPrefix(spec, "file://") || scpLike.MatchString(spec)
	if strings.HasPrefix(spec, "http://") {
		return Source{}, fmt.Errorf("plain http:// git URLs are not accepted; use https://")
	}
	if !isGit {
		return Source{Dir: spec}, nil
	}
	s := Source{}
	rest := spec
	if i := strings.LastIndex(rest, "#"); i >= 0 {
		s.Subdir, rest = rest[i+1:], rest[:i]
	}
	if i := refSeparator(rest); i >= 0 {
		s.Ref, rest = rest[i+1:], rest[:i]
	}
	if strings.HasPrefix(rest, "-") || strings.HasPrefix(s.Ref, "-") {
		return Source{}, fmt.Errorf("invalid git source %q", spec)
	}
	s.URL = rest
	if s.Subdir != "" {
		if err := checkSubdir(s.Subdir); err != nil {
			return Source{}, err
		}
	}
	return s, nil
}

// refSeparator finds the @ that starts a ref, as opposed to the user part of an
// ssh or https URL.
func refSeparator(s string) int {
	i := strings.LastIndex(s, "@")
	if i < 0 {
		return -1
	}
	if scpLike.MatchString(s) {
		if colon := strings.Index(s, ":"); i < colon {
			return -1
		}
		return i
	}
	if u, err := url.Parse(s[:i]); err == nil && (u.Host != "" || u.Scheme == "file") && u.Path != "" {
		return i
	}
	return -1
}

func checkSubdir(sub string) error {
	for _, seg := range strings.Split(sub, "/") {
		if seg == ".." || strings.ContainsRune(seg, 0) {
			return fmt.Errorf("invalid bundle subdirectory %q", sub)
		}
	}
	if filepath.IsAbs(sub) {
		return fmt.Errorf("the bundle subdirectory must be relative, got %q", sub)
	}
	return nil
}

// Fetch makes the bundle available as a local directory. For a git source it
// does a shallow fetch of ref into a temporary directory; cleanup removes it.
// Git runs without hooks, prompts or submodules.
func (s Source) Fetch(ctx context.Context) (dir string, cleanup func(), err error) {
	noop := func() {}
	if s.URL == "" {
		return s.Dir, noop, nil
	}
	if err := gitutil.CheckArg("bundle url", s.URL); err != nil {
		return "", noop, err
	}
	if err := gitutil.CheckArg("bundle ref", s.Ref); err != nil {
		return "", noop, err
	}
	tmp, err := os.MkdirTemp("", "ai-rulez-okf-*")
	if err != nil {
		return "", noop, fmt.Errorf("create temp dir: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(tmp) } //nolint:errcheck // best-effort temp cleanup
	ctx, cancel := context.WithTimeout(ctx, cloneTimeout)
	defer cancel()
	ref := s.Ref
	if ref == "" {
		ref = "HEAD"
	}
	for _, args := range [][]string{
		{"init", quiet},
		{"fetch", quiet, "--depth", "1", "--no-tags", "--no-recurse-submodules", "--", s.URL, ref},
		{"-c", "advice.detachedHead=false", "checkout", quiet, "--detach", "FETCH_HEAD"},
	} {
		if err := git(ctx, tmp, args...); err != nil {
			cleanup()
			return "", noop, err
		}
	}
	root := tmp
	if s.Subdir != "" {
		var err error
		if root, err = safeSubdir(tmp, s.Subdir); err != nil {
			cleanup()
			return "", noop, err
		}
	}
	return root, cleanup, nil
}

// safeSubdir resolves subdir below base and refuses a path that passes through a
// symlink: a fetched repository must not be able to point the bundle root at host
// files.
func safeSubdir(base, subdir string) (string, error) {
	cur := base
	for _, seg := range strings.Split(filepath.ToSlash(subdir), "/") {
		if seg == "" || seg == "." {
			continue
		}
		cur = filepath.Join(cur, seg)
		info, err := os.Lstat(cur)
		switch {
		case err != nil:
			return "", fmt.Errorf("bundle subdirectory %q: %w", subdir, err)
		case info.Mode()&os.ModeSymlink != 0:
			return "", fmt.Errorf("bundle subdirectory %q is or passes through a symlink, which is not followed", subdir)
		}
	}
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", base, err)
	}
	realCur, err := filepath.EvalSymlinks(cur)
	if err != nil {
		return "", fmt.Errorf("bundle subdirectory %q: %w", subdir, err)
	}
	if rel, err := filepath.Rel(realBase, realCur); err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("invalid bundle subdirectory %q", subdir)
	}
	return cur, nil
}

func git(ctx context.Context, dir string, args ...string) error {
	// -C dir (rather than a process working directory) keeps the call free of ambient state.
	res := gitutil.New(runner.FromContext(ctx)).Exec(ctx, dir, gitutil.HardenedEnv(nil), append(gitutil.HardenedConfig(), args...)...)
	if err := gitutil.ResultErr(res); err != nil {
		return fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(res.Stdout)+string(res.Stderr)))
	}
	return nil
}
