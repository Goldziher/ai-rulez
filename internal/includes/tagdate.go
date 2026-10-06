package includes

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/samber/oops"
)

// RemoteTagDate returns the date of a tag as the remote git data states it: the
// tagger date of an annotated tag, the committer date of a lightweight one. It
// fetches the tag (one commit deep, no blobs of interest are read) into a
// throwaway bare repository, so it costs a small fetch per tag. The date is
// whatever the pusher wrote: it can be forged, which is why it is the last
// source `min_release_age` tries.
func RemoteTagDate(ctx context.Context, repoURL, token, tag string) (time.Time, error) {
	if err := checkRemoteArgs(repoURL, ""); err != nil {
		return time.Time{}, err
	}
	if !safeTagRef(tag) {
		return time.Time{}, oops.With("tag", tag).Errorf("refusing to fetch a tag with an unusual name")
	}
	if err := requireGit(ctx); err != nil {
		return time.Time{}, err
	}
	dir, err := os.MkdirTemp("", "ai-rulez-tagdate-")
	if err != nil {
		return time.Time{}, oops.Wrapf(err, "create a scratch repository")
	}
	defer os.RemoveAll(dir) //nolint:errcheck // scratch directory
	env := withAuth(ctx, gitEnvFor(ctx), repoURL, token)
	if _, err := runGit(ctx, env, "init", "--quiet", "--bare", "--", dir); err != nil {
		return time.Time{}, err
	}
	ref := "refs/tags/" + tag
	res := gitRun(ctx, dir, env, "fetch", "--quiet", "--depth=1", "--no-tags", "--", repoURL, "+"+ref+":"+ref)
	if err := gitutil.ResultErr(res); err != nil {
		return time.Time{}, oops.With("url", RedactURL(repoURL)).With("output", RedactURL(strings.TrimSpace(string(res.Stderr)))).Wrapf(err, "fetch tag %q", tag)
	}
	res = gitRun(ctx, dir, env, "for-each-ref", "--format=%(creatordate:unix)", "--", ref)
	if err := gitutil.ResultErr(res); err != nil {
		return time.Time{}, oops.Wrapf(err, "read the date of tag %q", tag)
	}
	secs, err := strconv.ParseInt(strings.TrimSpace(string(res.Stdout)), 10, 64)
	if err != nil || secs <= 0 {
		return time.Time{}, oops.Errorf("tag %q has no date", tag)
	}
	return time.Unix(secs, 0).UTC(), nil
}

// safeTagRef reports whether a tag name is safe to put in a refspec: no
// characters git forbids in ref names, so a hostile remote cannot smuggle a
// second refspec or an option through a tag name.
func safeTagRef(tag string) bool {
	if tag == "" || len(tag) > 255 || strings.HasPrefix(tag, "-") || strings.HasPrefix(tag, "/") || strings.HasSuffix(tag, "/") ||
		strings.HasSuffix(tag, ".") || strings.HasSuffix(tag, ".lock") || strings.Contains(tag, "..") || strings.Contains(tag, "@{") || strings.Contains(tag, "//") {
		return false
	}
	for _, r := range tag {
		if r < 0x21 || r == 0x7f || strings.ContainsRune(`:~^?*[\`, r) {
			return false
		}
	}
	return true
}
