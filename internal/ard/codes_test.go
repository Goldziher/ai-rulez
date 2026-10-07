package ard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCodeMapsEveryRule(t *testing.T) {
	assert.Equal(t, CodeSchema, Code(RuleSchema))
	assert.Equal(t, CodeIdentifier, Code(RuleIdentifier))
	assert.Equal(t, CodeEntry, Code(RuleEntry))
	assert.Equal(t, CodeQueries, Code(RuleQueries))
	assert.Equal(t, CodeSchema, Code("something-else"))
}
