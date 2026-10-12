package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyPlugin_NamesEachDriftedPath(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, dir string)
		want   []PluginDrift
	}{
		{
			name: "edited file is changed",
			mutate: func(t *testing.T, dir string) {
				appendTo(t, filepath.Join(dir, "skills", "core-s", "SKILL.md"), "x\n")
			},
			want: []PluginDrift{{Path: "skills/core-s/SKILL.md", Kind: PluginDriftChanged}},
		},
		{
			name: "deleted file is missing",
			mutate: func(t *testing.T, dir string) {
				require.NoError(t, os.Remove(filepath.Join(dir, "skills", "core-s", "SKILL.md")))
			},
			want: []PluginDrift{{Path: "skills/core-s/SKILL.md", Kind: PluginDriftMissing}},
		},
		{
			name: "removed skill leaves an obsolete file",
			mutate: func(t *testing.T, dir string) {
				removeCoreSkill(t, dir)
			},
			want: []PluginDrift{
				{Path: ".cursor-plugin/skills/core-s/SKILL.md", Kind: PluginDriftObsolete},
				{Path: "skills/core-s/SKILL.md", Kind: PluginDriftObsolete},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir, _ := pruneProject(t)
			tt.mutate(t, dir)

			// Act
			err := loadDomainsProject(t, dir).VerifyPlugin("")

			// Assert
			require.ErrorIs(t, err, ErrPluginDrift)
			assert.Subset(t, PluginDriftOf(err), tt.want)
		})
	}
}

func TestPluginDriftOf_NilForOtherErrors(t *testing.T) {
	assert.Nil(t, PluginDriftOf(nil))
	assert.Nil(t, PluginDriftOf(os.ErrNotExist))
}
