package policy

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnforcerLocksInSeesTheOrganizationLayer(t *testing.T) {
	// Arrange: the owner's policy forbids both network features; nothing else does
	body := "policy_version = 1\n[telemetry]\nallow_network = false\n[llm]\nallow_network = false\n"
	f := newOrg(t)
	f.body.Store(body)
	user := "[policy]\ndiscover = \"org\"\n[policy.digests]\nexample-org = \"" + digest([]byte(body)) + "\"\n"
	base := f.opts(t, user)
	e := NewEnforcer(func() DiscoverOptions { return base })
	project := t.TempDir()

	// Act and Assert
	assert.False(t, e.Locks("telemetry"), "without a repository there is no owner to discover")
	assert.True(t, e.LocksIn("telemetry", project))
	assert.True(t, e.LocksIn("llm", project))
	assert.False(t, e.LocksIn("unknown", project))
	assert.False(t, e.LocksIn("telemetry", ""), "no directory, no discovery")
}

func TestEnforcerLocksInFailsClosedWhenTheOrganizationPolicyCannotBeRead(t *testing.T) {
	// Arrange: discovery is demanded and the owner's host answers with an error
	f := newOrg(t)
	f.code.Store(http.StatusBadGateway)
	base := f.opts(t, orgConfig(""))
	e := NewEnforcer(func() DiscoverOptions { return base })

	// Act and Assert
	assert.True(t, e.LocksIn("telemetry", t.TempDir()), "an unusable policy locks the network features")
	assert.True(t, e.LocksIn("llm", t.TempDir()))
}

func TestEnforcerLocksInWithoutDiscoveryIsLocks(t *testing.T) {
	// Arrange
	f := newOrg(t)
	base := f.opts(t, "")
	e := NewEnforcer(func() DiscoverOptions { return base })

	// Act and Assert
	assert.False(t, e.LocksIn("telemetry", t.TempDir()))
	assert.Zero(t, f.hits.Load())
}
