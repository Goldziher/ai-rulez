package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// A multi-skill entry resolves to names that no longer match the entry name, so
// the resolver stamps the entry's domain, profiles and trust on each file and
// the loader must prefer those over the name-keyed lookups.
func TestResolveInstalledSkillsPrefersStampedProvenance(t *testing.T) {
	// Arrange
	rec := &testutil.LogRecorder{}
	cfg := &Config{
		Host: ambient.Host{Log: rec},
		InstalledSkills: []InstalledSkillConfig{
			{
				Name:     "repo",
				Source:   "x",
				Domain:   "billing",
				Profiles: []string{"billing-team"},
				Trust:    TrustWarn,
			},
		},
		Content: &ContentTree{Domains: map[string]*Domain{"billing": {Name: "billing"}}},
	}
	resolve := func(context.Context, *Config) ([]ContentFile, error) {
		return []ContentFile{
			{Name: "team-refunds", Content: "a", Domain: "billing", Trust: TrustWarn, Profiles: []string{"billing-team"}},
			{Name: "team-credits", Content: "b", Domain: "billing", Trust: TrustWarn, Profiles: []string{"billing-team"}},
		}, nil
	}

	// Act
	err := resolveInstalledSkillsIfNeeded(logger.WithContext(context.Background(), rec), cfg, resolve)

	// Assert
	require.NoError(t, err)
	require.Contains(t, cfg.Content.Domains, "billing")
	skills := cfg.Content.Domains["billing"].Skills
	assert.Equal(t, []string{"team-refunds", "team-credits"}, contentFileNames(skills))
	assert.Empty(t, cfg.Content.Skills, "a stamped domain keeps the skills out of the root")
	for i := range skills {
		assert.Equal(t, []string{"billing-team"}, skills[i].Profiles)
		assert.Equal(t, TrustWarn, skills[i].Trust)
		assert.Equal(t, "billing", skills[i].Domain)
	}
}

// A resolver that does not stamp (a single-skill entry or a test double) must
// still get its profile scope and domain from the entry name.
func TestResolveInstalledSkillsFallsBackToNameLookupsWhenUnstamped(t *testing.T) {
	// Arrange
	cfg := &Config{
		InstalledSkills: []InstalledSkillConfig{
			{Name: "refunds", Source: "x", Domain: "billing", Profiles: []string{"billing-team"}},
		},
		Content: &ContentTree{Domains: map[string]*Domain{"billing": {Name: "billing"}}},
	}
	resolve := func(context.Context, *Config) ([]ContentFile, error) {
		return []ContentFile{{Name: "refunds", Content: "body"}}, nil
	}

	// Act
	err := resolveInstalledSkillsIfNeeded(context.Background(), cfg, resolve)

	// Assert
	require.NoError(t, err)
	require.Contains(t, cfg.Content.Domains, "billing")
	skills := cfg.Content.Domains["billing"].Skills
	require.Len(t, skills, 1)
	assert.Equal(t, []string{"billing-team"}, skills[0].Profiles)
}
