package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
	"github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
)

func TestApprove_ForbidSelfApprovalUsesTheIdentityAKeyIsMappedTo(t *testing.T) {
	// Arrange: the trusted approval key belongs to t@example.com, who also committed the change
	table := "forbid_self_approval = true\n\n[signing]\n[[signing.trust]]\nsubject = \"approval\"\nkey_file = \"keys/dave.pub\"\nreviewer = \"t@example.com\"\n"
	root := approveProject(t, table)
	_, pub, err := sigstore.GenerateKeyPair(nil)
	require.NoError(t, err)
	writeFile(t, filepath.Join(root, "keys", "dave.pub"), string(pub))
	key, err := signing.ParsePublicKey(pub)
	require.NoError(t, err)
	fingerprint, err := signing.Fingerprint(key)
	require.NoError(t, err)
	crossGit(t, root, "init", "-q", "-b", "main")
	crossGit(t, root, "add", "-A")
	crossGit(t, root, "commit", "-qm", "base")
	crossGit(t, root, "branch", "base")
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nUse spaces.\n")
	crossGit(t, root, "add", "-A")
	crossGit(t, root, "commit", "-qm", "change the rule")
	cfg := mustLoadConfig(t)
	lock, err := lockfile.Load(cfg.ConfigDir)
	require.NoError(t, err)
	var digest string
	for _, it := range lock.Item {
		if it.Kind == "rule" && it.ID == "style" {
			digest = it.Digest
		}
	}
	require.NotEmpty(t, digest)
	lock.Approval = append(lock.Approval,
		lockfile.Approval{Kind: "rule", ID: "style", Digest: digest, Reviewer: "key:" + fingerprint, Assurance: lockfile.AssuranceSigned, ApprovedAt: "2026-10-01T00:00:00Z"})

	// Act
	found, err := authorSelfApprovals(t.Context(), cfg, lock, "base")

	// Assert: matched as a self-approval by the mapped identity, not the "names no author" refusal
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "t@example.com", found[0].Author)
	assert.NotContains(t, found[0].Message(), "names no author")
}
