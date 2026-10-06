package approval

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/forge"
)

func TestApprovingReviews_OnlyMaintainersCountUnlessNamed(t *testing.T) {
	tests := []struct {
		name        string
		association string
		named       func(string) bool
		want        []string
	}{
		{"an owner counts", forge.AssociationOwner, nil, []string{"alice"}},
		{"a member counts", forge.AssociationMember, nil, []string{"alice"}},
		{"a collaborator counts", forge.AssociationCollaborator, nil, []string{"alice"}},
		{"a contributor is an outsider", "CONTRIBUTOR", nil, nil},
		{"a drive-by with no association is an outsider", "NONE", nil, nil},
		{"an unreported association is an outsider", "", nil, nil},
		{"an outsider the policy names counts", "NONE", func(login string) bool { return login == "alice" }, []string{"alice"}},
		{"an outsider the policy names someone else for does not", "NONE", func(login string) bool { return login == "bob" }, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			r := review(1, "alice", forge.ReviewApproved, headSHA, 1)
			r.AuthorAssociation = tt.association
			c := fakeForge(r)
			q := ReviewQuery{Repo: testRepo, PR: 7, Digest: approvedDigest, PinnedAt: pinned(map[string]string{headSHA: approvedDigest}), Named: tt.named}

			// Act
			got, err := ApprovingReviews(context.Background(), c, q)

			// Assert
			require.NoError(t, err)
			var logins []string
			for _, a := range got {
				logins = append(logins, a.Login)
			}
			assert.Equal(t, tt.want, logins)
		})
	}
}
