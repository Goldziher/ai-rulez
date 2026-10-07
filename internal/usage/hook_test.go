package usage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShellQuote_KeepsCommandSubstitutionLiteral(t *testing.T) {
	assert.Equal(t, `'$(id)'`, shellQuote("$(id)"))
	assert.Equal(t, "'a`b'", shellQuote("a`b"))
	assert.Equal(t, `"${CLAUDE_PROJECT_DIR}/x"`, shellQuote("${CLAUDE_PROJECT_DIR}/x"))
}
