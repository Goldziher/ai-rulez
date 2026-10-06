package forge

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Fake is an in-memory Client for tests of code that consumes a forge. Fill the
// maps before use; a missing entry is ErrNotFound. Err, when set, is returned by
// every method (offline, rate limited, ...). It records the calls it served.
type Fake struct {
	// Err fails every call when set.
	Err error
	// Releases by repo "host/owner/name", newest first as the forge lists them.
	ReleasesBy map[string][]Release
	// Tags by "repo@tag".
	Tags map[string]TagInfo
	// Commits by "repo@sha": the committer date.
	Commits map[string]time.Time
	// PRs by "repo@sha".
	PRs map[string][]PullRequest
	// Reviews by "repo#number".
	ReviewsBy map[string][]Review
	// Owners by "repo@ref" ("" ref is the default branch).
	Owners map[string]Codeowners
	// Teams by "host/org/slug": the member logins.
	Teams map[string][]string

	mu    sync.Mutex
	calls []string
}

var _ Client = (*Fake)(nil)

// Calls returns the recorded calls ("Releases host/o/n", ...), in order.
func (f *Fake) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *Fake) record(format string, args ...any) error {
	f.mu.Lock()
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
	f.mu.Unlock()
	return f.Err
}

// Releases implements Client.
func (f *Fake) Releases(_ context.Context, repo Repo) ([]Release, error) {
	if err := f.record("Releases %s", repo); err != nil {
		return nil, err
	}
	return append([]Release(nil), f.ReleasesBy[repo.String()]...), nil
}

// Release implements Client.
func (f *Fake) Release(_ context.Context, repo Repo, tag string) (Release, error) {
	if err := f.record("Release %s %s", repo, tag); err != nil {
		return Release{}, err
	}
	for _, r := range f.ReleasesBy[repo.String()] {
		if r.Tag == tag {
			return r, nil
		}
	}
	return Release{}, ErrNotFound
}

// Tag implements Client.
func (f *Fake) Tag(_ context.Context, repo Repo, tag string) (TagInfo, error) {
	if err := f.record("Tag %s %s", repo, tag); err != nil {
		return TagInfo{}, err
	}
	if t, ok := f.Tags[repo.String()+"@"+tag]; ok {
		return t, nil
	}
	return TagInfo{}, ErrNotFound
}

// CommitDate implements Client.
func (f *Fake) CommitDate(_ context.Context, repo Repo, sha string) (time.Time, error) {
	if err := f.record("CommitDate %s %s", repo, sha); err != nil {
		return time.Time{}, err
	}
	if t, ok := f.Commits[repo.String()+"@"+sha]; ok {
		return t, nil
	}
	return time.Time{}, ErrNotFound
}

// PullRequestsForCommit implements Client.
func (f *Fake) PullRequestsForCommit(_ context.Context, repo Repo, sha string) ([]PullRequest, error) {
	if err := f.record("PullRequestsForCommit %s %s", repo, sha); err != nil {
		return nil, err
	}
	return append([]PullRequest(nil), f.PRs[repo.String()+"@"+sha]...), nil
}

// Reviews implements Client.
func (f *Fake) Reviews(_ context.Context, repo Repo, pr int) ([]Review, error) {
	if err := f.record("Reviews %s %d", repo, pr); err != nil {
		return nil, err
	}
	return append([]Review(nil), f.ReviewsBy[fmt.Sprintf("%s#%d", repo, pr)]...), nil
}

// Codeowners implements Client.
func (f *Fake) Codeowners(_ context.Context, repo Repo, ref string) (Codeowners, error) {
	if err := f.record("Codeowners %s %s", repo, ref); err != nil {
		return Codeowners{}, err
	}
	if c, ok := f.Owners[repo.String()+"@"+ref]; ok {
		return c, nil
	}
	return Codeowners{}, ErrNotFound
}

// TeamMembers implements Client.
func (f *Fake) TeamMembers(_ context.Context, team Team) ([]string, error) {
	if err := f.record("TeamMembers %s", team); err != nil {
		return nil, err
	}
	m, ok := f.Teams[team.String()]
	if !ok {
		return nil, ErrNotFound
	}
	out := append([]string(nil), m...)
	sort.Strings(out)
	return out, nil
}

// IsTeamMember implements Client.
func (f *Fake) IsTeamMember(ctx context.Context, team Team, login string) (bool, error) {
	if err := f.record("IsTeamMember %s %s", team, login); err != nil {
		return false, err
	}
	for _, m := range f.Teams[team.String()] {
		if m == login {
			return true, nil
		}
	}
	return false, nil
}
