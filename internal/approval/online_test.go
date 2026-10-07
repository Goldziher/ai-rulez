package approval

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/forge"
)

var (
	testRepo = forge.Repo{Host: "github.com", Owner: "acme", Name: "config"}
	headSHA  = "1111111111111111111111111111111111111111"
	oldSHA   = "2222222222222222222222222222222222222222"
	baseTime = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
)

const approvedDigest = "sha256:aaaa"

func review(id int64, login, state, commit string, minutes int) forge.Review {
	return forge.Review{ID: id, Login: login, State: state, CommitID: commit, Submitted: baseTime.Add(time.Duration(minutes) * time.Minute), AuthorAssociation: forge.AssociationCollaborator}
}

// fakeForge serves pull request 7 (authored by dave), whose final head is headSHA.
func fakeForge(reviews ...forge.Review) *forge.Fake {
	return &forge.Fake{
		PRByNumber: map[string]forge.PullRequest{testRepo.String() + "#7": {Number: 7, Author: "dave", HeadSHA: headSHA}},
		ReviewsBy:  map[string][]forge.Review{testRepo.String() + "#7": reviews},
	}
}

// pinned answers PinnedAt from a commit-to-digest table.
func pinned(table map[string]string) func(context.Context, string) (string, bool, error) {
	return func(_ context.Context, sha string) (string, bool, error) {
		d, ok := table[sha]
		return d, ok, nil
	}
}

func TestApprovingReviews(t *testing.T) {
	sameEverywhere := map[string]string{headSHA: approvedDigest, oldSHA: approvedDigest}
	tests := []struct {
		name    string
		reviews []forge.Review
		pins    map[string]string
		want    []string
	}{
		{"an approval of content with the approved digest", []forge.Review{review(1, "alice", forge.ReviewApproved, headSHA, 1)}, sameEverywhere, []string{"alice"}},
		{"changes requested later withdraws it",
			[]forge.Review{review(1, "alice", forge.ReviewApproved, headSHA, 1), review(2, "alice", forge.ReviewChangesRequested, headSHA, 2)}, sameEverywhere, nil},
		{"a dismissed review is no approval", []forge.Review{review(1, "alice", forge.ReviewDismissed, headSHA, 1)}, sameEverywhere, nil},
		{"a later comment does not withdraw an approval",
			[]forge.Review{review(1, "alice", forge.ReviewApproved, headSHA, 1), review(2, "alice", forge.ReviewCommented, headSHA, 2)}, sameEverywhere, []string{"alice"}},
		{"approving again after changes requested counts",
			[]forge.Review{review(1, "alice", forge.ReviewChangesRequested, headSHA, 1), review(2, "alice", forge.ReviewApproved, headSHA, 2)}, sameEverywhere, []string{"alice"}},
		{"a review of an earlier head does not count, even for the same digest",
			[]forge.Review{review(1, "alice", forge.ReviewApproved, oldSHA, 1)}, sameEverywhere, nil},
		{"a re-review of the final head counts after an earlier-head one",
			[]forge.Review{review(1, "alice", forge.ReviewApproved, oldSHA, 1), review(2, "alice", forge.ReviewApproved, headSHA, 2)}, sameEverywhere, []string{"alice"}},
		{"the final head whose content has another digest approved something else",
			[]forge.Review{review(1, "alice", forge.ReviewApproved, headSHA, 1)}, map[string]string{headSHA: "sha256:bbbb"}, nil},
		{"a final head that does not pin the content", []forge.Review{review(1, "alice", forge.ReviewApproved, headSHA, 1)}, map[string]string{oldSHA: approvedDigest}, nil},
		{"two reviewers", []forge.Review{review(1, "bob", forge.ReviewApproved, headSHA, 1), review(2, "Alice", forge.ReviewApproved, headSHA, 2)}, sameEverywhere, []string{"Alice", "bob"}},
		{"the pending state never approves", []forge.Review{review(1, "alice", forge.ReviewPending, headSHA, 1)}, sameEverywhere, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			c := fakeForge(tt.reviews...)

			// Act
			got, err := ApprovingReviews(context.Background(), c, ReviewQuery{Repo: testRepo, PR: 7, Digest: approvedDigest, PinnedAt: pinned(tt.pins)})

			// Assert
			require.NoError(t, err)
			var logins []string
			for _, r := range got {
				logins = append(logins, r.Login)
			}
			assert.Equal(t, tt.want, logins)
		})
	}
}

func TestApprovingReviews_Failures(t *testing.T) {
	approved := review(1, "alice", forge.ReviewApproved, headSHA, 1)
	q := ReviewQuery{Repo: testRepo, PR: 7, Digest: approvedDigest, PinnedAt: pinned(map[string]string{headSHA: approvedDigest})}

	t.Run("a pull request the forge does not know is an error", func(t *testing.T) {
		c := fakeForge(approved)
		c.PRByNumber = nil
		_, err := ApprovingReviews(context.Background(), c, q)
		assert.ErrorIs(t, err, forge.ErrNotFound)
	})
	t.Run("a pull request without a head commit approves nothing", func(t *testing.T) {
		c := fakeForge(approved)
		c.PRByNumber = map[string]forge.PullRequest{testRepo.String() + "#7": {Number: 7, Author: "dave"}}
		got, err := ApprovingReviews(context.Background(), c, q)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
	t.Run("an empty digest never matches an unpinned commit", func(t *testing.T) {
		q := q
		q.Digest = ""
		q.PinnedAt = pinned(nil)
		got, err := ApprovingReviews(context.Background(), fakeForge(approved), q)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
	t.Run("a truncated review list is never counted", func(t *testing.T) {
		c := fakeForge(approved)
		c.Err = forge.ErrTruncated
		_, err := ApprovingReviews(context.Background(), c, q)
		assert.ErrorIs(t, err, forge.ErrTruncated)
	})
	t.Run("a lock that cannot be read at the reviewed commit is an error", func(t *testing.T) {
		q := q
		q.PinnedAt = func(context.Context, string) (string, bool, error) { return "", false, errors.New("unknown revision") }
		_, err := ApprovingReviews(context.Background(), fakeForge(approved), q)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown revision")
	})
}

func TestApprovingReviews_FlagsTheAuthor(t *testing.T) {
	c := fakeForge(review(1, "Dave", forge.ReviewApproved, headSHA, 1))

	got, err := ApprovingReviews(context.Background(), c, ReviewQuery{Repo: testRepo, PR: 7, Digest: approvedDigest, PinnedAt: pinned(map[string]string{headSHA: approvedDigest})})

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.True(t, got[0].Author, "a pull request author reviewing their own change")
	assert.Equal(t, "github:Dave", got[0].Reviewer())
	assert.Equal(t, "https://github.com/acme/config/pull/7#pullrequestreview-1", got[0].URL)
	assert.Equal(t, headSHA, got[0].CommitID)
}

func TestParseReviewRef(t *testing.T) {
	repo, pr, id, err := ParseReviewRef("https://github.com/acme/config/pull/7#pullrequestreview-123")
	require.NoError(t, err)
	assert.Equal(t, testRepo, repo)
	assert.Equal(t, 7, pr)
	assert.Equal(t, int64(123), id)
	assert.Equal(t, "https://github.com/acme/config/pull/7#pullrequestreview-123", ReviewURL(repo, pr, id))

	for _, bad := range []string{
		"", "http://github.com/acme/config/pull/7#pullrequestreview-1", "https://github.com/acme/config/issues/7#pullrequestreview-1",
		"https://github.com/acme/config/pull/x#pullrequestreview-1", "https://github.com/acme/config/pull/7", "https://github.com/acme/config/pull/7#issuecomment-4",
		"https://github.com/acme/pull/7#pullrequestreview-1", "https://github.com/ac me/config/pull/7#pullrequestreview-1",
	} {
		_, _, _, err := ParseReviewRef(bad)
		assert.Error(t, err, bad)
	}
}

func TestVerifyReviewRecord(t *testing.T) {
	ref := ReviewURL(testRepo, 7, 1)
	q := ReviewQuery{Repo: testRepo, Digest: approvedDigest, PinnedAt: pinned(map[string]string{headSHA: approvedDigest})}
	tests := []struct {
		name     string
		reviews  []forge.Review
		reviewer string
		ref      string
		wantErr  string
	}{
		{"still approved", []forge.Review{review(1, "alice", forge.ReviewApproved, headSHA, 1)}, "github:alice", ref, ""},
		{"approved, recorded as @login", []forge.Review{review(1, "alice", forge.ReviewApproved, headSHA, 1)}, "@alice", ref, ""},
		{"dismissed since", []forge.Review{review(1, "alice", forge.ReviewDismissed, headSHA, 1)}, "github:alice", ref, "no longer has an approving review"},
		{"another reviewer approved", []forge.Review{review(1, "bob", forge.ReviewApproved, headSHA, 1)}, "github:alice", ref, "no longer has an approving review"},
		{"the same repository in another letter case", []forge.Review{review(1, "alice", forge.ReviewApproved, headSHA, 1)}, "github:alice",
			ReviewURL(forge.Repo{Host: testRepo.Host, Owner: strings.ToUpper(testRepo.Owner), Name: strings.ToUpper(testRepo.Name)}, 7, 1), ""},
		{"another repository", nil, "github:alice", "https://github.com/other/config/pull/7#pullrequestreview-1", "not this repository"},
		{"not a link", nil, "github:alice", "see PR", "not an https review link"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := VerifyReviewRecord(context.Background(), fakeForge(tt.reviews...), tt.reviewer, tt.ref, q)

			// Assert
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestResolveTeams(t *testing.T) {
	c := &forge.Fake{Teams: map[string][]string{"github.com/acme/security": {"carol", "alice"}}}

	got, err := ResolveTeams(context.Background(), c, "github.com", []string{"@Acme/Security", "@acme/security", "@dave", "alice@example.org"})

	require.NoError(t, err)
	assert.Equal(t, map[string][]string{"@acme/security": {"alice", "carol"}}, got)
	assert.Len(t, c.Calls(), 1, "a team is read once")

	_, err = ResolveTeams(context.Background(), c, "github.com", []string{"@acme/missing"})
	assert.ErrorIs(t, err, forge.ErrNotFound)

	c.Err = forge.ErrForbidden
	_, err = ResolveTeams(context.Background(), c, "github.com", []string{"@acme/security"})
	assert.ErrorIs(t, err, forge.ErrForbidden)
}

func TestAuthorIs(t *testing.T) {
	tests := []struct {
		reviewer, email string
		want            bool
	}{
		{"alice@example.org", "Alice@Example.org", true},
		{"github:alice", "alice@users.noreply.github.com", true},
		{"@alice", "12345+alice@users.noreply.github.com", true},
		{"github:alice", "12345+bob@users.noreply.github.com", false},
		{"alice", "alice@example.org", false},
		{"", "", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, AuthorIs(tt.reviewer, tt.email), tt.reviewer+" vs "+tt.email)
	}
	msg := SelfApproval{Approval: rec("rule", "style", digestA, "alice@example.org"), Ref: "rule:style", Author: "alice@example.org"}.Message()
	assert.Contains(t, msg, "forbid_self_approval")
	assert.Contains(t, msg, "rule:style")
}

func TestApprovingReviews_RequiresAWriteRole(t *testing.T) {
	tests := []struct {
		name  string
		perms map[string]string
		named func(string) bool
		want  []string
	}{
		{"write, maintain and admin approve", map[string]string{"alice": "write", "bob": "maintain", "carol": "admin"}, nil, []string{"alice", "bob", "carol"}},
		{"read and triage roles approve nothing, though MEMBER or COLLABORATOR", map[string]string{"alice": "read", "bob": "triage", "carol": "write"}, nil, []string{"carol"}},
		{"a login the forge does not know is no maintainer", map[string]string{"carol": "write"}, nil, []string{"carol"}},
		{"a reviewer the policy names needs no role", map[string]string{}, func(l string) bool { return l == "alice" }, []string{"alice"}},
		{"a token that cannot read roles falls back to the author association", nil, nil, []string{"alice", "bob", "carol"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			c := fakeForge(
				review(1, "alice", forge.ReviewApproved, headSHA, 1),
				review(2, "bob", forge.ReviewApproved, headSHA, 2),
				review(3, "carol", forge.ReviewApproved, headSHA, 3),
			)
			if tt.perms != nil {
				c.Permissions = map[string]string{}
				for login, role := range tt.perms {
					c.Permissions[testRepo.String()+"@"+login] = role
				}
			}
			q := ReviewQuery{Repo: testRepo, PR: 7, Digest: approvedDigest, PinnedAt: pinned(map[string]string{headSHA: approvedDigest}), Named: tt.named}

			// Act
			got, err := ApprovingReviews(context.Background(), c, q)

			// Assert
			require.NoError(t, err)
			var logins []string
			for _, r := range got {
				logins = append(logins, r.Login)
			}
			assert.Equal(t, tt.want, logins)
		})
	}
	t.Run("another forge failure is an error, not a guess", func(t *testing.T) {
		c := fakeForge(review(1, "alice", forge.ReviewApproved, headSHA, 1))
		c.Permissions = map[string]string{}
		q := ReviewQuery{Repo: testRepo, PR: 7, Digest: approvedDigest, PinnedAt: pinned(map[string]string{headSHA: approvedDigest})}
		c.Err = nil
		failing := &roleFails{Fake: c}
		_, err := ApprovingReviews(context.Background(), failing, q)
		assert.ErrorIs(t, err, forge.ErrRateLimited)
	})
}

// roleFails is a forge whose role lookup is rate limited.
type roleFails struct{ *forge.Fake }

func (roleFails) CollaboratorPermission(context.Context, forge.Repo, string) (string, error) {
	return "", forge.ErrRateLimited
}
