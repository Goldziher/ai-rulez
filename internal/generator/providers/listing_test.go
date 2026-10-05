package providers_test

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadProviderSpec_Listing(t *testing.T) {
	t.Parallel()

	spec, err := providers.LoadProviderSpec([]byte(`
name = "demo"
[listing]
skills = true
include_path = true
description_limit = 500
[root]
file = "AGENTS.md"
sections = ["header"]
`), "demo.toml", providers.FormatAuto)
	require.NoError(t, err)
	require.NotNil(t, spec.Listing)
	assert.True(t, spec.Listing.Skills)
	assert.True(t, spec.Listing.IncludePath)
	assert.Equal(t, 500, spec.Listing.DescriptionLimit)
	assert.False(t, spec.Listing.Agents)
}

func TestBuiltinProviders_DeclareListing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		agents, includePath bool
		limit               int
	}{
		{"claude", true, false, 1536},
		{"junie", false, false, 0},
		{"pi", false, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen, err := providers.LoadBuiltin(tt.name)
			require.NoError(t, err)
			require.NotNil(t, gen.Spec.Listing)
			assert.True(t, gen.Spec.Listing.Skills)
			assert.Equal(t, tt.agents, gen.Spec.Listing.Agents)
			assert.Equal(t, tt.includePath, gen.Spec.Listing.IncludePath)
			assert.Equal(t, tt.limit, gen.Spec.Listing.DescriptionLimit)
		})
	}
}
