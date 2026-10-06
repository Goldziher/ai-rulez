package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestParseMinReleaseAgeSource(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{"the forge", "forge", "forge", false},
		{"first seen, any case", " First-Seen ", "first-seen", false},
		{"auto is no floor", "auto", "", true},
		{"commit is no floor", "commit", "", true},
		{"unknown", "registry", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, p, err := Parse("p.toml", []byte("policy_version = 1\n[sources]\nmin_release_age_source = \""+tt.value+"\"\n"))

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "min_release_age_source")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, p.Sources.MinReleaseAgeSource)
		})
	}
}

func TestMergeMinReleaseAgeSourceTakesTheStrongerSource(t *testing.T) {
	forge, seen, none := Sources{MinReleaseAgeSource: "forge"}, Sources{MinReleaseAgeSource: "first-seen"}, Sources{}

	assert.Equal(t, "forge", Merge(Policy{Sources: forge}, Policy{Sources: seen}).Sources.MinReleaseAgeSource)
	assert.Equal(t, "forge", Merge(Policy{Sources: seen}, Policy{Sources: forge}).Sources.MinReleaseAgeSource)
	assert.Equal(t, "first-seen", Merge(Policy{Sources: none}, Policy{Sources: seen}).Sources.MinReleaseAgeSource)
	assert.Equal(t, "", Merge(Policy{Sources: none}, Policy{Sources: none}).Sources.MinReleaseAgeSource)
}

func TestApplyMinReleaseAgeSource(t *testing.T) {
	tests := []struct {
		name     string
		floor    string
		have     string
		want     string
		wantViol []string
	}{
		{"unset takes the floor silently", "forge", "", "forge", nil},
		{"unset under first-seen is left to auto", "first-seen", "", "", nil},
		{"auto satisfies first-seen", "first-seen", "auto", "auto", nil},
		{"forge satisfies first-seen", "first-seen", "forge", "forge", nil},
		{"first-seen satisfies first-seen", "first-seen", "first-seen", "first-seen", nil},
		{"commit under first-seen is raised", "first-seen", "commit", "first-seen", []string{"AR740 lock.min_release_age_source"}},
		{"auto under forge is raised (it can fall back to first-seen)", "forge", "auto", "forge", []string{"AR740 lock.min_release_age_source"}},
		{"first-seen under forge is raised", "forge", "first-seen", "forge", []string{"AR740 lock.min_release_age_source"}},
		{"commit under forge is raised", "forge", "commit", "forge", []string{"AR740 lock.min_release_age_source"}},
		{"forge under forge", "forge", "forge", "forge", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := testConfig(t, "name = \"x\"\n")
			cfg.Lock = &config.LockConfig{MinReleaseAgeSource: tt.have}
			res := Resolve([]Layer{layer("managed", Policy{Sources: Sources{MinReleaseAgeSource: tt.floor}})})

			// Act
			result := res.Apply(cfg)

			// Assert
			assert.Equal(t, tt.want, cfg.Lock.MinReleaseAgeSource)
			assert.ElementsMatch(t, tt.wantViol, codes(result.Outcome))
		})
	}
}

func TestApplyMinReleaseAgeSourceWithoutALockTable(t *testing.T) {
	// Arrange
	cfg := testConfig(t, "name = \"x\"\n")
	res := Resolve([]Layer{layer("managed", Policy{Sources: Sources{MinReleaseAgeSource: "forge"}})})

	// Act
	result := res.Apply(cfg)

	// Assert
	require.NotNil(t, cfg.Lock)
	assert.Equal(t, "forge", cfg.Lock.MinReleaseAgeSource)
	assert.Empty(t, result.Outcome.Violations)
}

func TestLoosensReportsAWeakerReleaseSource(t *testing.T) {
	parent := Policy{Sources: Sources{MinReleaseAgeSource: "forge"}}

	assert.Contains(t, Loosens(parent, Policy{Sources: Sources{MinReleaseAgeSource: "first-seen"}}), "sources.min_release_age_source: \"first-seen\" is weaker than the parent's \"forge\"")
	assert.Empty(t, Loosens(parent, Policy{Sources: Sources{MinReleaseAgeSource: "forge"}}))
	assert.Empty(t, Loosens(Policy{Sources: Sources{MinReleaseAgeSource: "first-seen"}}, parent))
}
