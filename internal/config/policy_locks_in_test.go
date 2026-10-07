package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// dirLockEnforcer locks a feature only for one project directory, like an
// organization policy discovered from that repository's owner.
type dirLockEnforcer struct {
	fakeEnforcer
	dir string
}

func (d *dirLockEnforcer) LocksIn(feature, dir string) bool { return dir == d.dir && feature == "llm" }

func TestPolicyLocksInUsesTheProjectsOrganizationPolicy(t *testing.T) {
	// Arrange
	e := &dirLockEnforcer{dir: "/work/acme"}

	// Act and Assert
	assert.True(t, PolicyLocksIn(e, "llm", "/work/acme"))
	assert.False(t, PolicyLocksIn(e, "llm", "/work/other"))
	assert.False(t, PolicyLocksIn(e, "telemetry", "/work/acme"))
	assert.False(t, PolicyLocks(e, "llm"), "the answer without a project knows no repository")
}

func TestPolicyLocksInFallsBackToLocksForAnEnforcerThatCannotDiscover(t *testing.T) {
	// Arrange
	e := &fakeEnforcer{locked: map[string]bool{"llm": true}}

	// Act and Assert
	assert.True(t, PolicyLocksIn(e, "llm", "/work/acme"))
	assert.False(t, PolicyLocksIn(e, "telemetry", "/work/acme"))
}

func TestPolicyLocksInWithoutAnEnforcerLocksNothing(t *testing.T) {
	// Act and Assert
	assert.False(t, PolicyLocksIn(nil, "llm", "/work/acme"))
}
