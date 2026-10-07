package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestApprove_ForbidSelfApprovalRefusesAKeySignerThatNamesNoAuthor(t *testing.T) {
	// Arrange: a signed record whose reviewer is key:<id> cannot be matched with a commit author
	root := approveProject(t, "forbid_self_approval = true\n")
	crossGit(t, root, "init", "-q", "-b", "main")
	crossGit(t, root, "add", "-A")
	crossGit(t, root, "commit", "-qm", "base")
	cfg := mustLoadConfig(t)
	lock, err := lockfile.Load(cfg.ConfigDir)
	require.NoError(t, err)
	var item lockfile.Item
	for _, it := range lock.Item {
		if it.Kind == "rule" && it.ID == "style" {
			item = it
		}
	}
	require.NotEmpty(t, item.Digest)
	lock.Approval = append(lock.Approval, lockfile.Approval{Kind: "rule", ID: "style", Digest: item.Digest,
		Reviewer: "key:sha256:abc", Assurance: lockfile.AssuranceSigned, ApprovedAt: "2026-10-01T00:00:00Z"})

	// Act
	found, err := authorSelfApprovals(t.Context(), cfg, lock, "main")

	// Assert
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Contains(t, found[0].Message(), "key that names no author")
}

func TestApprove_VerifyBaseReadsOwnersFromTheBase(t *testing.T) {
	// Arrange: bob is made an owner and approves in the same range
	root := approveProject(t, "approvers_from = \"CODEOWNERS\"\n")
	writeFile(t, filepath.Join(root, "CODEOWNERS"), "* @alice\n")
	crossGit(t, root, "init", "-q", "-b", "main")
	crossGit(t, root, "add", "-A")
	crossGit(t, root, "commit", "-qm", "base")
	writeFile(t, filepath.Join(root, "CODEOWNERS"), "* @alice @bob\n")
	approveYes, approveReviewer = true, "bob"
	require.Equal(t, 0, mustApprove(t, "rule:style"))
	resetApproveFlags()
	approveVerifyBase = "main"

	// Act
	code, stdout, stderr := runApproveCmd(t)

	// Assert
	assert.Equal(t, 2, code, stderr)
	assert.Contains(t, stdout, "not by an owner of it in CODEOWNERS at")
}

func TestApprove_VerifyBaseFlagsAChangeToCodeownersAndGovernance(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, root string)
		want   string
	}{
		{"CODEOWNERS edited in the range", func(t *testing.T, root string) {
			writeFile(t, filepath.Join(root, "CODEOWNERS"), "* @alice @mallory\n")
		}, "CODEOWNERS changed"},
		{"governance table edited in the range", func(t *testing.T, root string) {
			p := filepath.Join(root, ".ai-rulez", "config.toml")
			data, err := os.ReadFile(p)
			require.NoError(t, err)
			writeFile(t, p, strings.Replace(string(data), "enforce = true", "enforce = true\nmin_approvers = 1", 1))
		}, "[governance] table"},
		{"nothing relevant changed", func(t *testing.T, root string) {}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := approveProject(t, "approvers_from = \"CODEOWNERS\"\n")
			writeFile(t, filepath.Join(root, "CODEOWNERS"), "* @alice\n")
			crossGit(t, root, "init", "-q", "-b", "main")
			crossGit(t, root, "add", "-A")
			crossGit(t, root, "commit", "-qm", "base")
			tt.change(t, root)
			approveVerifyBase = "main"

			// Act
			code, stdout, stderr := runApproveCmd(t)

			// Assert
			if tt.want == "" {
				assert.Equal(t, 0, code, stderr)
				return
			}
			assert.Equal(t, 2, code, stderr)
			assert.Contains(t, stdout, "AR716")
			assert.Contains(t, stdout, tt.want)
		})
	}
}
