package approval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestPolicyOf_UnreadableLockFailsClosedInsteadOfDroppingTheDenyList(t *testing.T) {
	// Arrange: a lock that does not parse holds the deny list nobody can read
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, lockfile.FileName), []byte("this is = not [ toml"), 0o600))
	cfg := &config.Config{ConfigDir: dir, Governance: &config.GovernanceConfig{RequireApproval: []string{"remote"}}}
	include := Subject{Kind: KindInclude, ID: "shared", Digest: digestB, Class: ClassRemote}

	// Act
	p := PolicyOf(cfg)
	res := p.Evaluate([]lockfile.Approval{rec("include", "shared", digestB, "alice")}, include, testNow)

	// Assert
	assert.Equal(t, StatusDenied, res.Status)
	assert.Contains(t, res.Detail, "deny list")
}
