package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// attestedProject approves rule:style, then points the record at a bundle file.
func attestedProject(t *testing.T, bundle string) (root, bundlePath string) {
	t.Helper()
	root = approveProject(t, "")
	approveYes, approveReviewer = true, "alice@example.org"
	mustApprove(t, "rule:style")
	resetApproveFlags()
	cfg := mustLoadConfig(t)
	lock, err := lockfile.Load(cfg.ConfigDir)
	require.NoError(t, err)
	digest := approval.BundleDigest([]byte(bundle))
	lock.Approval[0].Attestation = digest
	require.NoError(t, lockfile.Save(cfg.ConfigDir, lock))
	bundlePath, err = approval.AttestationFile(cfg.ConfigDir, digest)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(bundlePath), 0o750))
	require.NoError(t, os.WriteFile(bundlePath, []byte(bundle), 0o600))
	return root, bundlePath
}

func TestApprove_RevokeAndPruneRemoveTheAttestationBundle(t *testing.T) {
	tests := []struct {
		name string
		flag func()
		args []string
	}{
		{"revoke", func() { approveRevoke = true }, []string{"rule:style"}},
		{"prune of a stale record", func() { approvePrune = true }, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root, bundlePath := attestedProject(t, "{\"bundle\":\""+tt.name+"\"}")
			if tt.name != "revoke" {
				writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nchanged\n")
				require.Equal(t, 0, writeLockAt("", "", nil))
			}
			tt.flag()

			// Act
			code, _, stderr := runApproveCmd(t, tt.args...)

			// Assert
			require.Equal(t, 0, code, stderr)
			assert.NoFileExists(t, bundlePath)
		})
	}
}

func TestApprove_RevokeKeepsABundleAnotherRecordStillNames(t *testing.T) {
	// Arrange: two records share one bundle
	_, bundlePath := attestedProject(t, "{\"bundle\":\"shared\"}")
	cfg := mustLoadConfig(t)
	lock, err := lockfile.Load(cfg.ConfigDir)
	require.NoError(t, err)
	second := lock.Approval[0]
	second.Reviewer = "bob@example.org"
	lock.Approval = append(lock.Approval, second)
	require.NoError(t, lockfile.Save(cfg.ConfigDir, lock))
	approveRevoke, approveReviewer = true, "alice@example.org"

	// Act
	code, _, stderr := runApproveCmd(t, "rule:style")

	// Assert
	require.Equal(t, 0, code, stderr)
	assert.FileExists(t, bundlePath)
}

func TestApprove_RevokeByGithubPrefixedReviewer(t *testing.T) {
	// Arrange
	root := approveProject(t, "")
	approveYes, approveReviewer = true, "alice"
	mustApprove(t, "rule:style")
	resetApproveFlags()
	approveRevoke, approveReviewer = true, "github:Alice"

	// Act
	code, stdout, stderr := runApproveCmd(t, "rule:style")

	// Assert
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stdout, "revoked 1")
	assert.NotContains(t, lockText(t, root), "[[approval]]")
}

func TestApprove_DenyReasonIsScannedForSecrets(t *testing.T) {
	// Arrange
	approveProject(t, "")
	approveYes, approveReviewer = true, "alice"
	mustApprove(t, "rule:style")
	resetApproveFlags()
	approveRevoke, approveDeny = true, true
	approveReason = "leaked AKIAIOSFODNN7EXAMPLE and ghp_abcdefghijklmnopqrstuvwxyz0123456789"

	// Act
	code, _, stderr := runApproveCmd(t, "rule:style")

	// Assert
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "--reason looks like it contains a secret")
}
