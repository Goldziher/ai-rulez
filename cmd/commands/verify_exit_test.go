package commands

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator"

	"github.com/stretchr/testify/assert"
)

func TestPluginVerifyExitCode(t *testing.T) {
	assert.Equal(t, exitDrift, pluginVerifyExitCode(fmt.Errorf("wrapped: %w", generator.ErrPluginDrift)))
	assert.Equal(t, 1, pluginVerifyExitCode(errors.New("render expected plugin outputs")))
}
