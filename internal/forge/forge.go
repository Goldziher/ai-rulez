// Package forge is a minimal, read-only client for the hosting forge of a git
// source (GitHub and GitHub Enterprise): release and tag dates, commit dates,
// pull request reviews, CODEOWNERS and team membership.
//
// Two features rest on it: `min_release_age` (docs/lockfile.md) reads the
// forge's publish time of a tag, which a committer cannot forge, and approvals
// (internal/approval) read reviews, CODEOWNERS and team membership. Client is
// the interface they depend on; HTTPClient implements it for GitHub, Fake is an
// in-memory implementation, and forgetest serves the GitHub endpoints over TLS.
//
// Safety properties, all enforced by HTTPClient and tested:
//   - the token comes only from the environment (GITHUB_TOKEN, GH_TOKEN) or
//     `gh auth token` (fixed argv), never from a project file or a flag;
//   - a request goes only to a host on the forge allowlist
//     (AI_RULEZ_FORGE_HOSTS, default github.com), over https, and the token
//     never leaves that host: a redirect to any other host is refused;
//   - every response body is size capped, every listing page capped (a capped
//     listing returns what it has with ErrTruncated), and every identifier that
//     becomes part of a URL is validated;
//   - in offline mode no request is made and every method returns ErrOffline.
package forge

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Errors a Client returns. Test with errors.Is.
var (
	// ErrOffline: the client was built offline, so nothing was requested.
	ErrOffline = errors.New("forge lookups need the network and offline mode is on")
	// ErrNotFound: the forge has no such release, tag, commit, file or team (or the token cannot see it).
	ErrNotFound = errors.New("not found on the forge")
	// ErrHostNotAllowed: the repository's host is not on the token host allowlist, so it is never contacted.
	ErrHostNotAllowed = errors.New("host is not in the forge allowlist")
	// ErrUnsupportedSource: the source is not a repository on a supported forge (a local path, plain http, an unknown URL shape).
	ErrUnsupportedSource = errors.New("not a repository on a supported forge")
	// ErrUnauthorized: the forge rejected the credentials, or the call needs a token and there is none.
	ErrUnauthorized = errors.New("the forge needs a valid token for this lookup")
	// ErrForbidden: the token lacks the permission the call needs (read:org for team membership, for example).
	ErrForbidden = errors.New("the forge refused the request: the token lacks permission")
	// ErrRateLimited: the forge's rate limit is exhausted; RateLimitError carries the reset time.
	ErrRateLimited = errors.New("the forge rate limit is exhausted")
	// ErrTruncated: a listing hit the page cap; the items returned are the first ones only.
	ErrTruncated = errors.New("the forge listing was cut at the page cap")
	// ErrTooLarge: a response exceeded the size cap and was discarded.
	ErrTooLarge = errors.New("the forge response exceeds the size cap")
)

// RateLimitError is ErrRateLimited with the time the limit resets (zero if unknown).
type RateLimitError struct{ Reset time.Time }

func (e *RateLimitError) Error() string {
	if e.Reset.IsZero() {
		return ErrRateLimited.Error()
	}
	return fmt.Sprintf("%s (resets at %s)", ErrRateLimited, e.Reset.UTC().Format(time.RFC3339))
}

// Is makes errors.Is(err, ErrRateLimited) true.
func (e *RateLimitError) Is(target error) bool { return target == ErrRateLimited }

// Repo identifies a repository on a forge. Host is lower case ("github.com").
type Repo struct {
	Host, Owner, Name string
}

// String is "host/owner/name".
func (r Repo) String() string { return r.Host + "/" + r.Owner + "/" + r.Name }

var (
	hostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)
	nameRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
	userRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}(\[bot\])?$`)
	shaRe  = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
)

// Valid reports whether r can be used in a request path.
func (r Repo) Valid() error {
	switch {
	case !hostRe.MatchString(r.Host):
		return fmt.Errorf("%w: invalid host %q", ErrUnsupportedSource, r.Host)
	case !validName(r.Owner), !validName(r.Name):
		return fmt.Errorf("%w: invalid owner or name in %q", ErrUnsupportedSource, r.String())
	}
	return nil
}

func validName(s string) bool { return nameRe.MatchString(s) && s != "." && s != ".." }

// ParseRepo reads the repository out of a git source as written in a config:
// https://host/owner/name[.git], git+https://..., ssh://git@host/owner/name,
// and the scp form git@host:owner/name. A nested path (a GitLab subgroup, a
// ".../tree/main" suffix) is ErrUnsupportedSource, as are plain http, git://,
// file:// and local paths: only a repository whose identity is certain is looked up.
func ParseRepo(source string) (Repo, error) {
	s := strings.TrimSpace(source)
	var host, path string
	switch {
	case strings.Contains(s, "://"):
		u, err := url.Parse(strings.TrimPrefix(s, "git+"))
		if err != nil {
			return Repo{}, fmt.Errorf("%w: %q", ErrUnsupportedSource, redact(s))
		}
		switch strings.ToLower(u.Scheme) {
		case "https", "ssh":
		default:
			return Repo{}, fmt.Errorf("%w: scheme %q", ErrUnsupportedSource, u.Scheme)
		}
		host, path = u.Hostname(), u.Path
	default:
		at, colon := strings.Index(s, "@"), strings.Index(s, ":")
		if at <= 0 || colon < at || strings.ContainsAny(s[:colon], `/\`) {
			return Repo{}, fmt.Errorf("%w: %q", ErrUnsupportedSource, redact(s))
		}
		host, path = s[at+1:colon], s[colon+1:]
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	owner, name, ok := strings.Cut(path, "/")
	if !ok || strings.Contains(name, "/") {
		return Repo{}, fmt.Errorf("%w: expected owner/name, got %q", ErrUnsupportedSource, path)
	}
	repo := Repo{Host: strings.ToLower(host), Owner: owner, Name: name}
	if err := repo.Valid(); err != nil {
		return Repo{}, err
	}
	return repo, nil
}

// redact drops userinfo from a URL-looking string before it appears in an error.
func redact(s string) string {
	if i := strings.Index(s, "://"); i >= 0 {
		rest := s[i+3:]
		if at := strings.Index(rest, "@"); at >= 0 && at < strings.Index(rest+"/", "/") {
			return s[:i+3] + rest[at+1:]
		}
	}
	return s
}

// Team is a team of an organization on a host: "@acme/reviewers" on github.com is
// Team{Host: "github.com", Org: "acme", Slug: "reviewers"}.
type Team struct{ Host, Org, Slug string }

// String is "host/org/slug".
func (t Team) String() string { return t.Host + "/" + t.Org + "/" + t.Slug }

// Valid reports whether t can be used in a request path.
func (t Team) Valid() error {
	if !hostRe.MatchString(t.Host) || !validName(t.Org) || !validName(t.Slug) {
		return fmt.Errorf("%w: invalid team %q", ErrUnsupportedSource, t.String())
	}
	return nil
}

// Release is a published release of a tag.
type Release struct {
	Tag        string
	Name       string
	Prerelease bool
	// Published is the time the forge published the release; the trusted release date.
	Published time.Time
}

// TagInfo describes a git tag as the forge knows it.
type TagInfo struct {
	Name string
	// Commit is the commit the tag resolves to (an annotated tag is peeled).
	Commit string
	// TagObject is the annotated tag object id; empty for a lightweight tag.
	TagObject string
	// Tagged is the tagger date of an annotated tag; zero for a lightweight tag.
	Tagged time.Time
}

// PullRequest is the part of a pull request approvals need.
type PullRequest struct {
	Number int
	State  string // "open" or "closed"
	// Merged is true once the pull request was merged.
	Merged bool
	// Author is the login of the author.
	Author string
	// BaseRef is the branch it targets; HeadSHA the tip of its head branch.
	BaseRef string
	HeadSHA string
	// MergeCommit is the commit the merge produced, "" until merged.
	MergeCommit string
}

// Review states, as GitHub reports them.
const (
	ReviewApproved         = "APPROVED"
	ReviewChangesRequested = "CHANGES_REQUESTED"
	ReviewCommented        = "COMMENTED"
	ReviewDismissed        = "DISMISSED"
	ReviewPending          = "PENDING"
)

// Review is one pull request review.
type Review struct {
	ID    int64
	Login string
	State string
	// CommitID is the head commit the reviewer saw. An approval counts for a
	// change only when CommitID is the commit the change landed at (or the PR's
	// final head): a review of an earlier head approved something else.
	CommitID  string
	Submitted time.Time
}

// Codeowners is a CODEOWNERS file.
type Codeowners struct {
	// Path is where it was found: ".github/CODEOWNERS", "CODEOWNERS" or "docs/CODEOWNERS".
	Path    string
	Content []byte
}

// Client is what the rest of ai-rulez needs from a forge. Every method takes
// the Repo (or org) explicitly, so one client serves many sources.
//
// Listings (Releases, PullRequestsForCommit, Reviews, TeamMembers) return the
// items read so far together with ErrTruncated when the page cap was hit: the
// caller decides whether a partial answer is acceptable (for approvals it is
// not: count only a complete review list).
type Client interface {
	// Releases lists the published (non-draft) releases, newest first.
	Releases(ctx context.Context, repo Repo) ([]Release, error)
	// Release returns the published release of tag, or ErrNotFound (a tag
	// without a release has none: use TagDate).
	Release(ctx context.Context, repo Repo, tag string) (Release, error)
	// Tag resolves a tag to its commit and, for an annotated tag, its tagger date.
	Tag(ctx context.Context, repo Repo, tag string) (TagInfo, error)
	// CommitDate is the committer date of a commit. It is whatever the committer
	// wrote: anyone who can push can forge it.
	CommitDate(ctx context.Context, repo Repo, sha string) (time.Time, error)
	// PullRequestsForCommit lists the pull requests that contain a commit.
	PullRequestsForCommit(ctx context.Context, repo Repo, sha string) ([]PullRequest, error)
	// PullRequest returns one pull request by number, with the tip of its head
	// branch as it is now: the commit a review must have seen to count.
	PullRequest(ctx context.Context, repo Repo, number int) (PullRequest, error)
	// Reviews lists the reviews of a pull request, oldest first.
	Reviews(ctx context.Context, repo Repo, pr int) ([]Review, error)
	// Codeowners fetches the CODEOWNERS file at ref ("" is the default branch),
	// trying .github/, the root, then docs/. ErrNotFound when there is none.
	Codeowners(ctx context.Context, repo Repo, ref string) (Codeowners, error)
	// TeamMembers lists the logins of the members of the team. It
	// needs a token that can read the organization.
	TeamMembers(ctx context.Context, team Team) ([]string, error)
	// IsTeamMember reports whether login is an active member of the team.
	IsTeamMember(ctx context.Context, team Team, login string) (bool, error)
}
