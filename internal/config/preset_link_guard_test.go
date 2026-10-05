package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/providers"
)

// TestEmbeddedProviderPresetsRegisterWhenProvidersLinked guards the registration
// contract: config cannot import providers, so a spec-only preset is valid only
// because linking providers (which internal/generator does) registers it. If this
// fails, a spec name stopped reaching config.RegisterBuiltInPresetName.
func TestEmbeddedProviderPresetsRegisterWhenProvidersLinked(t *testing.T) {
	names, err := providers.BuiltinNames()
	require.NoError(t, err)
	registered := config.AllPresetNames()
	for _, name := range names {
		assert.Contains(t, registered, name)
		p := config.Preset{BuiltIn: name}
		assert.True(t, p.IsValid(), name)
	}
}
