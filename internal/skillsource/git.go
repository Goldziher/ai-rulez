package skillsource

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/samber/oops"
)

// refHEAD names the remote's default branch.
const refHEAD = "HEAD"

// Ref kinds reported by resolution.
const (
	kindSHA    = "commit"
	kindTag    = "tag"
	kindBranch = "branch"
	kindHead   = "head"
)

// checkRemote refuses a url or ref git would read as an option (for example
// `--upload-pack=<command>`), before any git command is built from it.
func checkRemote(url, ref string) error {
	if err := gitutil.CheckRemoteURL("skill source url", gitURL(url)); err != nil {
		return oops.Wrap(err)
	}
	if err := gitutil.CheckArg("skill source ref", ref); err != nil {
		return oops.Wrap(err)
	}
	return nil
}

// gitTimeout bounds one git command, so an unresponsive server cannot hang startup.
var gitTimeout = 5 * time.Minute

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	// Everything a skill source names is untrusted: no hooks, helpers or submodules,
	// and only the https, ssh and file transports (never ext::). Long paths: the
	// cache tree sits deep below the user cache directory, and git's own files
	// below it pass MAX_PATH on Windows (other platforms ignore the setting).
	argv := append(gitutil.HardenedConfig(), "-c", "core.longpaths=true")
	res := gitutil.New(runner.FromContext(ctx)).Exec(ctx, dir, gitutil.HardenedEnv(nil), append(argv, args...)...)
	if err := gitutil.ResultErr(res); err != nil {
		return "", oops.With("output", includes.RedactURL(strings.TrimSpace(string(res.Stderr)))).Wrapf(err, "git %s", args[0])
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}

// lsRemote resolves ref to a commit on the remote and says what kind of ref it
// was. Tags are preferred over branches (the peeled commit for annotated tags);
// an empty ref or HEAD resolves the default branch.
func lsRemote(ctx context.Context, url, ref string) (commit, kind string, err error) {
	if checkErr := checkRemote(url, ref); checkErr != nil {
		return "", "", checkErr
	}
	remote := url
	if ref == "" || ref == refHEAD {
		out, err := runGit(ctx, "", "ls-remote", "--", remote, refHEAD)
		if err != nil {
			return "", "", oops.With("url", includes.RedactURL(url)).Wrapf(err, "resolve default branch")
		}
		sha, _, _ := strings.Cut(out, "\t")
		if !gitutil.IsCommitSHA(sha) {
			return "", "", oops.With("url", includes.RedactURL(url)).Errorf("remote has no HEAD")
		}
		return sha, kindHead, nil
	}
	out, err := runGit(ctx, "", "ls-remote", "--", remote, "refs/tags/"+ref, "refs/tags/"+ref+"^{}", "refs/heads/"+ref)
	if err != nil {
		return "", "", oops.With("url", includes.RedactURL(url)).With("ref", ref).Wrapf(err, "resolve ref")
	}
	var tag, peeled, branch string
	for _, line := range strings.Split(out, "\n") {
		sha, name, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok {
			continue
		}
		switch name {
		case "refs/tags/" + ref:
			tag = sha
		case "refs/tags/" + ref + "^{}":
			peeled = sha
		case "refs/heads/" + ref:
			branch = sha
		}
	}
	switch {
	case peeled != "":
		return peeled, kindTag, nil
	case tag != "":
		return tag, kindTag, nil
	case branch != "":
		return branch, kindBranch, nil
	}
	return "", "", oops.With("url", includes.RedactURL(url)).With("ref", ref).Errorf("ref %q not found on the remote (expected a tag, a branch or a full commit SHA)", ref)
}

// fetchCommit materializes commit of url into dest (a fresh directory) and
// removes the git metadata, leaving only the tree. The commit is fetched by its
// SHA, so a tag or branch that moved since the lock pinned it does not matter.
// A server that refuses a fetch by SHA is asked for ref (the whole history, as
// the pinned commit may be behind the tip) and the commit must then be
// reachable; otherwise the fetch fails closed. The checkout is verified to be at
// commit. flagQuiet keeps git silent.
//
// The clone is a partial one (see cloneRequest.partialFilter), restricted to the
// source path with a sparse checkout when there is one, and it is aborted with
// ErrCloneTooLarge as soon as the destination grows past the size limit.
const flagQuiet = "--quiet"

func fetchCommit(ctx context.Context, req cloneRequest, dest string) error {
	if err := checkRemote(req.url, req.ref); err != nil {
		return err
	}
	if !gitutil.IsCommitSHA(req.commit) {
		return oops.Errorf("refusing to fetch %q: not a full commit SHA", req.commit)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return oops.Wrapf(err, "create checkout directory")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	watch := watchSize(dest, req, cancel)
	err := cloneInto(ctx, req, dest)
	watch.stop()
	if grown := watch.exceeded.Load(); grown != nil {
		return req.tooLarge(*grown)
	}
	if err != nil {
		return err
	}
	if grown := req.measure(dest); req.over(grown) {
		return req.tooLarge(grown)
	}
	// Dropping .git makes the cached tree immutable and smaller; the digest ignores it anyway.
	return oops.Wrapf(os.RemoveAll(filepath.Join(dest, ".git")), "drop git metadata")
}

func cloneInto(ctx context.Context, req cloneRequest, dest string) error {
	for _, args := range [][]string{
		{"init", flagQuiet},
		{"remote", "add", "origin", "--", req.url},
	} {
		if _, err := runGit(ctx, dest, args...); err != nil {
			return oops.With("url", includes.RedactURL(req.url)).Wrapf(err, "prepare checkout")
		}
	}
	if p := cleanSrcPath(req.path); p != "" && safeSparsePath(p) {
		// Cone mode takes the path literally (no glob patterns). An old git without
		// the command falls back to a full checkout, still bounded by the size limit.
		if _, err := runGit(ctx, dest, "sparse-checkout", "set", "--cone", "--", p); err != nil {
			logger.FromContext(ctx).Debug("git sparse-checkout is not available; checking out the whole repository", "error", err.Error())
		}
	}
	if _, err := runGit(ctx, dest, "fetch", flagQuiet, "--depth", "1", "--no-tags", req.partialFilter(), "--", "origin", req.commit); err != nil {
		if err = fetchViaRef(ctx, dest, req); err != nil {
			return oops.With("url", includes.RedactURL(req.url)).With("ref", req.ref).With("commit", req.commit).Wrap(err)
		}
	} else if _, err = runGit(ctx, dest, "checkout", flagQuiet, "--detach", "FETCH_HEAD"); err != nil {
		return oops.With("url", includes.RedactURL(req.url)).With("commit", req.commit).Wrapf(err, "check out the pinned commit")
	}
	head, err := runGit(ctx, dest, "rev-parse", refHEAD)
	if err != nil {
		return err
	}
	if head != req.commit {
		return oops.With("url", includes.RedactURL(req.url)).Errorf("fetched commit %s but %s was resolved; the remote served another commit", head, req.commit)
	}
	return nil
}

// safeSparsePath reports whether p can be given to `sparse-checkout set`.
func safeSparsePath(p string) bool {
	return p != ".." && !strings.HasPrefix(p, "../") && !strings.HasPrefix(p, "/") && !strings.ContainsAny(p, "\\\x00")
}

// fetchViaRef is the fallback for a server that will not serve a commit by SHA:
// it fetches the ref with its history and checks the commit out if it is in it.
func fetchViaRef(ctx context.Context, dest string, req cloneRequest) error {
	ref, kind, commit := req.ref, req.kind, req.commit
	if kind == kindSHA {
		return oops.Errorf("the server does not serve commit %s by SHA", commit)
	}
	name := ref
	if name == "" {
		name = refHEAD
	}
	if _, err := runGit(ctx, dest, "fetch", flagQuiet, "--no-tags", req.partialFilter(), "--", "origin", name); err != nil {
		return oops.Wrapf(err, "fetch %s", name)
	}
	if _, err := runGit(ctx, dest, "cat-file", "-e", commit+"^{commit}"); err != nil {
		return oops.Errorf("pinned commit %s is not reachable from %s any more (the ref was rewritten); it cannot be fetched, run `ai-rulez lock` after reviewing the new content", commit, refLabel(ref))
	}
	if _, err := runGit(ctx, dest, "checkout", flagQuiet, "--detach", commit); err != nil {
		return oops.Wrapf(err, "check out the pinned commit")
	}
	return nil
}
