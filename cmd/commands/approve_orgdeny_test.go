package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestApprove_RefusesADigestTheOrganizationPolicyDenies(t *testing.T) {
	// Arrange
	root := approveProject(t, "")
	lock, err := lockfile.Load(filepath.Join(root, ".ai-rulez"))
	require.NoError(t, err)
	var digest string
	for _, it := range lock.Item {
		if it.Kind == "rule" && it.ID == "style" {
			digest = it.Digest
		}
	}
	require.NotEmpty(t, digest)
	pol := filepath.Join(t.TempDir(), "policy.toml")
	writeFile(t, pol, "policy_version = 1\n[sources]\ndeny_digests = [\""+digest+"\"]\n")
	t.Setenv("AI_RULEZ_POLICY", pol)
	approveYes, approveReviewer = true, "alice@example.org"

	// Act
	code, _, stderr := runApproveCmd(t, "rule:style")

	// Assert
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "AR747")
}
