package scanners

import (
	"os"
	"path/filepath"
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

func TestSkillspectorProfileIsOptInAndSingleBatch(t *testing.T) {
	p, ok := Lookup("skillspector")
	require.True(t, ok)
	assert.False(t, p.Egress)
	assert.Equal(t, 1, p.BatchSize, "skillspector scan takes exactly one input path")
	assert.Equal(t, []string{"--no-llm"}, p.RequiredFlags)
	assert.Contains(t, p.Command, "--no-llm")
	assert.Contains(t, p.Command, "{skill_dirs}")
	assert.NotEmpty(t, p.Version)
	assert.Empty(t, PresetsOf("skillspector"), "a heuristic scanner stays out of the presets")
}

func TestRequiredFlagsAreInTheirOwnCommand(t *testing.T) {
	for _, p := range Profiles() {
		for _, f := range p.RequiredFlags {
			assert.Contains(t, p.Command, f, "%s: required flag %s", p.Name, f)
		}
	}
}

func TestSchemasListEveryProfile(t *testing.T) {
	for _, file := range []string{"ai-rules.schema.json", "ai-rules-local.schema.json"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "schema", file))
		require.NoError(t, err)
		for _, name := range Names() {
			assert.Contains(t, string(raw), `"`+name+`"`, "%s lists profile %s", file, name)
		}
	}
}
