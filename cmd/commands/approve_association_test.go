package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/forge"
)

func TestApprove_FromGithubReviewIgnoresOutsidersUnlessNamed(t *testing.T) {
	tests := []struct {
		name      string
		governing string
		codeowner string
		want      []string
	}{
		{name: "an outsider nobody names is dropped", want: []string{"github:alice"}},
		{name: "an outsider the allowlist names counts", governing: "\napprovers = [\"github:alice\", \"github:erin\"]\n", want: []string{"github:alice", "github:erin"}},
		{name: "an outsider CODEOWNERS names counts", governing: "\napprovers_from = \"CODEOWNERS\"\n", codeowner: "* @alice @erin\n", want: []string{"github:alice", "github:erin"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: alice is a member of the repository, erin a drive-by reviewer
			root, head, fake := reviewedProject(t, tt.governing)
			erin := ghReview(2, "erin", forge.ReviewApproved, head)
			erin.AuthorAssociation = "NONE"
			fake.ReviewsBy["github.com/acme/config#7"] = []forge.Review{ghReview(1, "alice", forge.ReviewApproved, head), erin}
			if tt.codeowner != "" {
				writeFile(t, filepath.Join(root, ".github", "CODEOWNERS"), tt.codeowner)
			}
			approveYes, approveFromReview = true, 7

			// Act
			code, _, stderr := runApproveCmd(t, "rule:style")

			// Assert
			require.Equal(t, 0, code, stderr)
			var got []string
			for _, a := range loadLockAt(t, root).Approval {
				got = append(got, a.Reviewer)
			}
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
