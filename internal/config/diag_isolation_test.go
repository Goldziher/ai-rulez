package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Two projects loaded in one process share no warning state: a message one of
// them already said once is still new for the other.
func TestLoadedConfigsHaveTheirOwnWarningState(t *testing.T) {
	// Arrange
	load := func() *Config {
		cfg, err := LoadConfig(t.Context(), "/virtual/proj", WithWorkspace(memProject()), WithoutRemote())
		require.NoError(t, err)
		return cfg
	}
	first, second := load(), load()

	// Act
	firstSaid := first.OnceKey("isolation-test-key")
	firstAgain := first.OnceKey("isolation-test-key")
	secondSaid := second.OnceKey("isolation-test-key")

	// Assert
	assert.True(t, firstSaid)
	assert.False(t, firstAgain)
	assert.True(t, secondSaid, "a second project must not inherit the first one's once-per-process state")
}
