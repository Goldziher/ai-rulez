package policy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// A snapshot of an old revision is loaded from a temporary directory, which has
// no remote: organization discovery has to ask the project the snapshot is of.
func TestEnforcerDiscoversTheOrgPolicyFromThePolicyDir(t *testing.T) {
	// Arrange
	f := newOrg(t)
	base := f.opts(t, orgConfig(""))
	var asked []string
	inner := base.RemoteURL
	base.RemoteURL = func(dir string) (string, error) {
		asked = append(asked, dir)
		return inner(dir)
	}
	e := NewEnforcer(func() DiscoverOptions { return base })
	project := t.TempDir()
	snapshot := t.TempDir()
	cfg := &config.Config{BaseDir: snapshot, PolicyDir: project, Lock: &config.LockConfig{Enforce: boolPtr(false)}}

	// Act
	out, err := e.Enforce(context.Background(), cfg)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{project}, asked, "discovery reads the remote of the project, not of the snapshot")
	require.NotNil(t, out)
	assert.Contains(t, codes(out), "AR740 lock.enforce")
}
