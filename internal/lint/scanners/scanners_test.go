package scanners

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEveryProfileIsComplete(t *testing.T) {
	for _, p := range Profiles() {
		t.Run(p.Name, func(t *testing.T) {
			assert.NotEmpty(t, p.Command, "command")
			assert.NotEmpty(t, p.Source, "source")
			assert.Regexp(t, `^\d{4}-\d{2}-\d{2}$`, p.VerifiedOn, "verified_on")
			assert.NotEmpty(t, p.Format, "format")
			if p.Egress {
				assert.NotEmpty(t, p.DataSent, "an egress profile names what the vendor receives")
			} else {
				assert.Empty(t, p.RequiresEnv, "an egress = false profile needs no credential")
			}
		})
	}
}

func TestPresetsContainOnlyKnownNonEgressProfiles(t *testing.T) {
	for _, name := range PresetNames() {
		preset, ok := LookupPreset(name)
		require.True(t, ok)
		require.NotEmpty(t, preset.Profiles, name)
		for _, member := range preset.Profiles {
			p, ok := Lookup(member)
			require.True(t, ok, "preset %s names the unknown profile %s", name, member)
			assert.False(t, p.Egress, "preset %s must never contain the egress scanner %s", name, member)
		}
	}
}

func TestNoNonEgressProfileCarriesAnEgressFlagInItsCommand(t *testing.T) {
	for _, p := range Profiles() {
		if p.Egress {
			continue
		}
		for _, arg := range p.Command {
			assert.NotContains(t, p.EgressFlags, arg, "%s: %s is its own deny-listed flag", p.Name, arg)
		}
	}
}

func TestPresetsOfAndLookups(t *testing.T) {
	assert.Equal(t, []string{"baseline", "strict"}, PresetsOf("agnix"))
	assert.Equal(t, []string{"strict"}, PresetsOf("cisco-skill-scanner"))
	assert.Empty(t, PresetsOf("snyk-agent-scan"))
	_, ok := Lookup("nope")
	assert.False(t, ok)
	_, ok = LookupPreset("off")
	assert.False(t, ok)
	strict, _ := LookupPreset("strict")
	assert.Equal(t, "warning", strict.FailOn)
}
