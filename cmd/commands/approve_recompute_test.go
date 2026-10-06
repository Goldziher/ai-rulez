package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/forge"
)

func TestApprove_FromGithubReviewRecomputesTheDigestAtTheReviewedCommit(t *testing.T) {
	// Arrange: the reviewed commit holds the old rule text but a lock that already
	// pins the new text, so trusting the lock at that commit would link a review of
	// content the reviewer never saw.
	root, _, fake := reviewedProject(t, "")
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nUse spaces.\n")
	require.Equal(t, 0, writeLockAt("", "", nil))
	crossGit(t, root, "add", ".ai-rulez/ai-rulez.lock")
	crossGit(t, root, "commit", "-qm", "lock only")
	head := crossGit(t, root, "rev-parse", "HEAD")
	fake.PRByNumber["github.com/acme/config#7"] = forge.PullRequest{Number: 7, Author: "dave", HeadSHA: head}
	fake.ReviewsBy["github.com/acme/config#7"] = []forge.Review{ghReview(1, "alice", forge.ReviewApproved, head)}
	approveYes, approveFromReview = true, 7

	// Act
	code, _, stderr := runApproveCmd(t, "rule:style")

	// Assert
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "no approving review of rule:style")
	assert.Empty(t, loadLockAt(t, root).Approval)
}
