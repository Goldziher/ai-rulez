package skillsource

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/gitutil"
	"github.com/Goldziher/ai-rulez/internal/includes"
	"github.com/samber/oops"
)

var fullSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

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
	if err := gitutil.CheckArg("skill source url", gitURL(url)); err != nil {
		return oops.Wrap(err)
	}
	if err := gitutil.CheckArg("skill source ref", ref); err != nil {
		return oops.Wrap(err)
	}
	return nil
}

func injectToken(u, token string) string {
	if token == "" {
		return u
	}
	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(u, scheme) {
			return scheme + token + ":x-oauth-basic@" + strings.TrimPrefix(u, scheme)
		}
	}
	return u
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	// Everything a skill source names is untrusted: no hooks, helpers or submodules,
	// and only the https, ssh and file transports (never ext::).
	cmd := gitutil.Command(ctx, dir, append(gitutil.HardenedConfig(), args...)...)
	cmd.Env = gitutil.HardenedEnv(nil)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return "", oops.With("output", includes.RedactURL(strings.TrimSpace(errOut.String()))).Wrapf(err, "git %s", args[0])
	}
	return strings.TrimSpace(out.String()), nil
}

// lsRemote resolves ref to a commit on the remote and says what kind of ref it
// was. Tags are preferred over branches (the peeled commit for annotated tags);
// an empty ref or HEAD resolves the default branch.
func lsRemote(ctx context.Context, url, ref, token string) (commit, kind string, err error) {
	if err = checkRemote(url, ref); err != nil {
		return "", "", err
	}
	remote := injectToken(url, token)
	if ref == "" || ref == "HEAD" {
		out, err := runGit(ctx, "", "ls-remote", "--", remote, "HEAD")
		if err != nil {
			return "", "", oops.With("url", includes.RedactURL(url)).Wrapf(err, "resolve default branch")
		}
		sha, _, _ := strings.Cut(out, "\t")
		if !fullSHA.MatchString(sha) {
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
// removes the git metadata, leaving only the tree. It verifies the checkout is
// at commit so a tag that moved between resolution and fetch fails closed.
// flagQuiet keeps git silent.
const flagQuiet = "--quiet"

func fetchCommit(ctx context.Context, url, ref, kind, commit, token, dest string) error {
	if err := checkRemote(url, ref); err != nil {
		return err
	}
	remote := injectToken(url, token)
	if kind == kindTag || kind == kindBranch {
		if _, err := runGit(ctx, "", "clone", flagQuiet, "--depth", "1", "--branch", ref, "--", remote, dest); err != nil {
			return oops.With("url", includes.RedactURL(url)).With("ref", ref).Wrapf(err, "clone")
		}
	} else {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return oops.Wrapf(err, "create checkout directory")
		}
		for _, args := range [][]string{
			{"init", flagQuiet},
			{"remote", "add", "origin", "--", remote},
			{"fetch", flagQuiet, "--depth", "1", "--", "origin", commit},
			{"checkout", flagQuiet, "--detach", "FETCH_HEAD"},
		} {
			if _, err := runGit(ctx, dest, args...); err != nil {
				return oops.With("url", includes.RedactURL(url)).With("commit", commit).Wrapf(err, "fetch pinned commit")
			}
		}
	}
	head, err := runGit(ctx, dest, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if head != commit {
		return oops.With("url", includes.RedactURL(url)).Errorf("fetched commit %s but %s was resolved; the ref moved during the fetch, retry", head, commit)
	}
	// Dropping .git makes the cached tree immutable and smaller; the digest ignores it anyway.
	return oops.Wrapf(os.RemoveAll(filepath.Join(dest, ".git")), "drop git metadata")
}
