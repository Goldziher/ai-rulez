package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRoot_EachCommandIsRegisteredOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range RootCmd.Commands() {
		assert.False(t, seen[c.Name()], "%q is registered twice", c.Name())
		seen[c.Name()] = true
	}
}
