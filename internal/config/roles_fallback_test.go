package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoleSkillModeFallback(t *testing.T) {
	base := "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n"
	tests := []struct {
		name    string
		table   string
		want    string
		wantErr bool
	}{
		{name: "unset defaults to drop", want: SkillModeFallbackDrop},
		{name: "drop", table: "[role_manifest]\nskill_mode_fallback = \"drop\"\n", want: SkillModeFallbackDrop},
		{name: "serve", table: "[role_manifest]\nskill_mode_fallback = \"serve\"\n", want: SkillModeFallbackServe},
		{name: "other values are rejected", table: "[role_manifest]\nskill_mode_fallback = \"hide\"\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := schemaProject(t, base+tt.table, "")

			// Act
			err := cfg.Validate()

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "skill_mode_fallback")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.RoleSkillModeFallback())
		})
	}
}
