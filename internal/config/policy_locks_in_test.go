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
	SetPolicyEnforcer(&dirLockEnforcer{dir: "/work/acme"})
	t.Cleanup(func() { SetPolicyEnforcer(nil) })

	// Act and Assert
	assert.True(t, PolicyLocksIn("llm", "/work/acme"))
	assert.False(t, PolicyLocksIn("llm", "/work/other"))
	assert.False(t, PolicyLocksIn("telemetry", "/work/acme"))
	assert.False(t, PolicyLocks("llm"), "the process-wide answer knows no repository")
}

func TestPolicyLocksInFallsBackToLocksForAnEnforcerThatCannotDiscover(t *testing.T) {
	// Arrange
	SetPolicyEnforcer(&fakeEnforcer{locked: map[string]bool{"llm": true}})
	t.Cleanup(func() { SetPolicyEnforcer(nil) })

	// Act and Assert
	assert.True(t, PolicyLocksIn("llm", "/work/acme"))
	assert.False(t, PolicyLocksIn("telemetry", "/work/acme"))
}

func TestPolicyLocksInWithoutAnEnforcerLocksNothing(t *testing.T) {
	// Arrange
	SetPolicyEnforcer(nil)

	// Act and Assert
	assert.False(t, PolicyLocksIn("llm", "/work/acme"))
}
