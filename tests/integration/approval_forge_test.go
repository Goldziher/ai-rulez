package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/forge/forgetest"
)

const (
	headSHA  = "1111111111111111111111111111111111111111"
	olderSHA = "2222222222222222222222222222222222222222"
	digestA  = "sha256:aaaa"
)

func review(id int64, login, state, commit, assoc string, at int) forge.Review {
	return forge.Review{ID: id, Login: login, State: state, CommitID: commit, AuthorAssociation: assoc,
		Submitted: time.Date(2026, 10, 1, 9, at, 0, 0, time.UTC)}
}

// TestReviewLinkedApprovalsAgainstAFakeForge is the manual pass's review-linked
// approvals against a forge fake: which approving reviews of a pull request
// count for the content at its head.
func TestReviewLinkedApprovalsAgainstAFakeForge(t *testing.T) {
	tests := []struct {
		name       string
		reviews    []forge.Review
		collab     map[string]string
		named      []string
		pinnedAt   string
		wantLogins []string
		wantAuthor []string
	}{
		{
			name:       "a collaborator with write access at the final head counts",
			reviews:    []forge.Review{review(1, "alice", forge.ReviewApproved, headSHA, "COLLABORATOR", 1)},
			collab:     map[string]string{"alice": "write"},
			pinnedAt:   digestA,
			wantLogins: []string{"alice"},
		},
		{
			name:     "a drive-by approval from outside the repository counts for nothing",
			reviews:  []forge.Review{review(1, "mallory", forge.ReviewApproved, headSHA, "NONE", 1)},
			collab:   map[string]string{"mallory": "write"},
			pinnedAt: digestA,
		},
		{
			name:     "an organization member who can only read counts for nothing",
			reviews:  []forge.Review{review(1, "bob", forge.ReviewApproved, headSHA, "MEMBER", 1)},
			collab:   map[string]string{"bob": "read"},
			pinnedAt: digestA,
		},
		{
			name:     "a review of an earlier head counts for nothing",
			reviews:  []forge.Review{review(1, "alice", forge.ReviewApproved, olderSHA, "COLLABORATOR", 1)},
			collab:   map[string]string{"alice": "write"},
			pinnedAt: digestA,
		},
		{
			name: "a later request for changes withdraws the approval",
			reviews: []forge.Review{
				review(1, "alice", forge.ReviewApproved, headSHA, "COLLABORATOR", 1),
				review(2, "alice", forge.ReviewChangesRequested, headSHA, "COLLABORATOR", 2),
			},
			collab:   map[string]string{"alice": "write"},
			pinnedAt: digestA,
		},
		{
			name:     "content at the head with another digest is not what was approved",
			reviews:  []forge.Review{review(1, "alice", forge.ReviewApproved, headSHA, "COLLABORATOR", 1)},
			collab:   map[string]string{"alice": "write"},
			pinnedAt: "sha256:bbbb",
		},
		{
			name:       "the pull request author's own approval is marked as the author's",
			reviews:    []forge.Review{review(1, "carol", forge.ReviewApproved, headSHA, "OWNER", 1)},
			collab:     map[string]string{"carol": "admin"},
			pinnedAt:   digestA,
			wantLogins: []string{"carol"},
			wantAuthor: []string{"carol"},
		},
		{
			name:       "a reviewer the policy names counts without a repository role",
			reviews:    []forge.Review{review(1, "dave", forge.ReviewApproved, headSHA, "CONTRIBUTOR", 1)},
			named:      []string{"dave"},
			pinnedAt:   digestA,
			wantLogins: []string{"dave"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			srv := forgetest.New(t)
			srv.PRs = map[string][]forge.PullRequest{headSHA: {{Number: 42, State: "open", Author: "carol", BaseRef: "main", HeadSHA: headSHA}}}
			srv.Reviews = map[int][]forge.Review{42: tt.reviews}
			srv.Collaborators = tt.collab
			client := srv.Client(map[string]string{"GITHUB_TOKEN": "t0ken"})
			q := approval.ReviewQuery{
				Repo: forge.Repo{Host: "github.com", Owner: "acme", Name: "skills"}, PR: 42, Digest: digestA,
				PinnedAt: func(context.Context, string) (string, bool, error) { return tt.pinnedAt, true, nil },
				Named: func(login string) bool {
					for _, n := range tt.named {
						if n == login {
							return true
						}
					}
					return false
				},
			}

			// Act
			got, err := approval.ApprovingReviews(context.Background(), client, q)

			// Assert
			require.NoError(t, err)
			var logins, authors []string
			for _, r := range got {
				logins = append(logins, r.Login)
				if r.Author {
					authors = append(authors, r.Login)
				}
			}
			assert.Equal(t, tt.wantLogins, logins)
			assert.Equal(t, tt.wantAuthor, authors)
		})
	}
}

// TestTeamResolutionFailsClosed: a team the forge cannot expand authorizes
// nobody instead of being skipped.
func TestTeamResolutionFailsClosed(t *testing.T) {
	// Arrange
	srv := forgetest.New(t)
	srv.Teams = map[string][]string{"acme/security": {"alice"}}
	client := srv.Client(map[string]string{"GITHUB_TOKEN": "t0ken"})

	// Act
	known, knownErr := approval.ResolveTeams(context.Background(), client, "github.com", []string{"@acme/security", "alice@example.org"})
	_, unknownErr := approval.ResolveTeams(context.Background(), client, "github.com", []string{"@acme/ghosts"})

	// Assert
	require.NoError(t, knownErr)
	assert.Equal(t, map[string][]string{"@acme/security": {"alice"}}, known)
	require.Error(t, unknownErr, "an unknown team is an error, not an empty team")
}
