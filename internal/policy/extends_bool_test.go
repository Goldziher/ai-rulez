package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtendsRejectsAChildThatSwitchesAParentRestrictionOff(t *testing.T) {
	tests := []struct {
		name, parent, kid, want string
	}{
		{"require_pinned", "[sources]\nrequire_pinned = true\n", "[sources]\nrequire_pinned = false\n", "sources.require_pinned"},
		{"lock enforce", "[lock]\nenforce = true\n", "[lock]\nenforce = false\n", "lock.enforce"},
		{"lock include_outputs", "[lock]\ninclude_outputs = true\n", "[lock]\ninclude_outputs = false\n", "lock.include_outputs"},
		{"telemetry network", "[telemetry]\nallow_network = false\n", "[telemetry]\nallow_network = true\n", "telemetry.allow_network"},
		{"llm network", "[llm]\nallow_network = false\n", "[llm]\nallow_network = true\n", "llm.allow_network"},
		{"guard", "[guard]\ngenerated = true\n", "[guard]\ngenerated = false\n", "guard.generated"},
		{"governance enforce", "[governance]\nenforce = true\n", "[governance]\nenforce = false\n", "governance.enforce"},
		{"forbid_self_approval", "[governance]\nforbid_self_approval = true\n", "[governance]\nforbid_self_approval = false\n", "governance.forbid_self_approval"},
		{"hooks", "[hooks]\nallow = false\n", "[hooks]\nallow = true\n", "hooks.allow"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			chainFile(t, dir, "org.toml", nil, tt.parent)
			team := chainFile(t, dir, "team.toml", []string{"org.toml"}, tt.kid)

			// Act
			_, err := Discover(extendsOpts(t, team))

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), "AR743")
			assert.Contains(t, err.Error(), "loosens")
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestExtendsAcceptsABooleanThatKeepsOrAddsARestriction(t *testing.T) {
	tests := []struct{ name, parent, kid string }{
		{"the same value", "[lock]\nenforce = true\n", "[lock]\nenforce = true\n"},
		{"a switch the parent leaves off", "[lock]\nenforce = false\n", "[lock]\nenforce = true\n"},
		{"false where the parent set nothing", "", "[lock]\nenforce = false\n"},
		{"false next to a parent that is off too", "[guard]\ngenerated = false\n", "[guard]\ngenerated = false\n"},
		{"a child that says nothing", "[sources]\nrequire_pinned = true\n", "[lint]\nrequired_codes = [\"AR001\"]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			chainFile(t, dir, "org.toml", nil, tt.parent)
			team := chainFile(t, dir, "team.toml", []string{"org.toml"}, tt.kid)

			// Act
			_, err := Discover(extendsOpts(t, team))

			// Assert
			require.NoError(t, err)
		})
	}
}
