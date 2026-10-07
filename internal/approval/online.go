package approval

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/forge"
)

// ReviewQuery says which pull request review to look for and which content it
// must have approved.
type ReviewQuery struct {
	Repo forge.Repo
	// PR is the pull request number.
	PR int
	// Digest is the digest of the content being approved. A review counts only
	// when the content at the pull request's final head has this digest: the
	// review was of exactly this content.
	Digest string
	// PinnedAt returns the digest of the content as it is at commit sha, and
	// whether the content exists there. A commit that is not in the local clone is
	// an error (fetch the pull request head).
	PinnedAt func(ctx context.Context, sha string) (digest string, pinned bool, err error)
	// Named reports whether the policy names the reviewer for this content
	// ([governance] approvers or CODEOWNERS). A reviewer who is not an owner,
	// member or collaborator of the repository counts only when named; nil names nobody.
	Named func(login string) bool
}

// ReviewApproval is one approving review that applies to the commit.
type ReviewApproval struct {
	Login    string
	ReviewID int64
	// URL points at the review on the forge.
	URL string
	// CommitID is the head commit the reviewer saw.
	CommitID string
	// Author reports that the reviewer is also the pull request author.
	Author bool
}

// Reviewer is the reviewer string a review-linked approval records.
func (r ReviewApproval) Reviewer() string { return "github:" + r.Login }

// ReviewURL is the link to one review of a pull request.
func ReviewURL(repo forge.Repo, pr int, review int64) string {
	return fmt.Sprintf("https://%s/%s/%s/pull/%d#pullrequestreview-%d", repo.Host, repo.Owner, repo.Name, pr, review)
}

// ParseReviewRef reads a review link written by ReviewURL.
func ParseReviewRef(ref string) (repo forge.Repo, pr int, review int64, err error) {
	u, err := url.Parse(strings.TrimSpace(ref))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return forge.Repo{}, 0, 0, fmt.Errorf("ref %q is not an https review link", ref)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 4 || parts[2] != "pull" {
		return forge.Repo{}, 0, 0, fmt.Errorf("ref %q is not a pull request link (https://host/owner/repo/pull/N#pullrequestreview-ID)", ref)
	}
	if pr, err = strconv.Atoi(parts[3]); err != nil || pr <= 0 {
		return forge.Repo{}, 0, 0, fmt.Errorf("ref %q has no pull request number", ref)
	}
	id, ok := strings.CutPrefix(u.Fragment, "pullrequestreview-")
	if !ok {
		return forge.Repo{}, 0, 0, fmt.Errorf("ref %q names no review (#pullrequestreview-ID)", ref)
	}
	if review, err = strconv.ParseInt(id, 10, 64); err != nil || review <= 0 {
		return forge.Repo{}, 0, 0, fmt.Errorf("ref %q has an invalid review id", ref)
	}
	repo = forge.Repo{Host: strings.ToLower(u.Hostname()), Owner: parts[0], Name: parts[1]}
	if err := repo.Valid(); err != nil {
		return forge.Repo{}, 0, 0, err //nolint:wrapcheck // names the invalid part
	}
	return repo, pr, review, nil
}

// ApprovingReviews returns the reviews that approve q.Digest: the latest
// decisive review of each reviewer is APPROVED (a later CHANGES_REQUESTED or
// DISMISSED withdraws it; comments neither approve nor withdraw), the reviewer is
// an owner, member or collaborator of the repository or named by the policy, the commit the
// reviewer saw is the pull request's final head (a review of an earlier head
// approved something else, even when the content looks the same), and the
// content at that commit has the digest q.Digest. Incomplete listings are
// errors: a truncated review list is never counted.
func ApprovingReviews(ctx context.Context, c forge.Client, q ReviewQuery) ([]ReviewApproval, error) {
	pr, err := c.PullRequest(ctx, q.Repo, q.PR)
	if err != nil {
		return nil, fmt.Errorf("read pull request #%d: %w", q.PR, err)
	}
	reviews, err := c.Reviews(ctx, q.Repo, q.PR)
	if err != nil {
		return nil, fmt.Errorf("list the reviews of #%d: %w", q.PR, err)
	}
	var out []ReviewApproval
	pinnedAtHead := ""
	checked := false
	for login, r := range latestDecisive(reviews) {
		if r.State != forge.ReviewApproved || r.CommitID == "" || pr.HeadSHA == "" || !strings.EqualFold(r.CommitID, pr.HeadSHA) {
			continue
		}
		if !r.Maintainer() && (q.Named == nil || !q.Named(r.Login)) {
			continue // anyone can review a public repository: an outsider approves nothing
		}
		if !checked {
			digest, pinned, err := q.PinnedAt(ctx, pr.HeadSHA)
			if err != nil {
				return nil, fmt.Errorf("read the content at the reviewed commit %s: %w", shortSHA(pr.HeadSHA), err)
			}
			if pinned {
				pinnedAtHead = digest
			}
			checked = true
		}
		if pinnedAtHead == "" || pinnedAtHead != q.Digest { // an unpinned commit has no digest to match
			continue
		}
		out = append(out, ReviewApproval{
			Login: r.Login, ReviewID: r.ID, URL: ReviewURL(q.Repo, q.PR, r.ID), CommitID: r.CommitID,
			Author: strings.EqualFold(login, pr.Author),
		})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Login) < strings.ToLower(out[j].Login) })
	return out, nil
}

// latestDecisive keeps, per lower-cased login, the latest review that approves or
// withdraws: comments and pending reviews neither approve nor withdraw.
func latestDecisive(reviews []forge.Review) map[string]forge.Review {
	latest := map[string]forge.Review{}
	for i := range reviews {
		r := reviews[i]
		switch r.State {
		case forge.ReviewApproved, forge.ReviewChangesRequested, forge.ReviewDismissed:
		default:
			continue
		}
		login := strings.ToLower(r.Login)
		if prev, ok := latest[login]; !ok || !r.Submitted.Before(prev.Submitted) {
			latest[login] = r
		}
	}
	return latest
}

// VerifyReviewRecord re-checks a review-linked record online: its ref must name
// a review of the repository's own pull request, and the reviewer must still
// approve q.Digest. q.PR is taken from the ref.
func VerifyReviewRecord(ctx context.Context, c forge.Client, reviewer, ref string, q ReviewQuery) error {
	repo, pr, _, err := ParseReviewRef(ref)
	if err != nil {
		return err
	}
	if !strings.EqualFold(repo.String(), q.Repo.String()) {
		return fmt.Errorf("ref points at %s, not this repository (%s)", repo, q.Repo)
	}
	q.PR = pr
	reviews, err := ApprovingReviews(ctx, c, q)
	if err != nil {
		return err
	}
	for _, r := range reviews {
		if SameReviewer(r.Reviewer(), reviewer) {
			return nil
		}
	}
	return fmt.Errorf("%s no longer has an approving review of #%d of this content (dismissed, changes requested, or made on other content)", reviewer, pr)
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// ResolveTeams reads the members of every team in entries from the forge and
// returns them keyed by lower-cased "@org/team". A team the forge cannot expand
// (no permission, not found) is an error: authorization stays closed rather than
// guess. Teams of another host than host are skipped by the caller.
func ResolveTeams(ctx context.Context, c forge.Client, host string, entries []string) (map[string][]string, error) {
	out := map[string][]string{}
	for _, e := range entries {
		if !IsTeam(e) {
			continue
		}
		key := teamKey(e)
		if _, done := out[key]; done {
			continue
		}
		org, slug, _ := strings.Cut(strings.TrimPrefix(key, "@"), "/")
		team := forge.Team{Host: host, Org: org, Slug: slug}
		if err := team.Valid(); err != nil {
			return nil, err //nolint:wrapcheck // names the invalid team
		}
		members, err := c.TeamMembers(ctx, team)
		if err != nil {
			return nil, fmt.Errorf("read the members of %s: %w", key, err)
		}
		out[key] = members
	}
	return out, nil
}
