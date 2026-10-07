package config

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_LintTolerateAndDeprecatedBudget(t *testing.T) {
	tests := []struct {
		name  string
		lint  string
		local string
		want  map[string]int
	}{
		{"new key", "[lint.ratchet]\nAR201 = 1\n", "", map[string]int{"AR201": 1}},
		{"deprecated alias still works", "[lint.budget]\nAR201 = 2\n", "", map[string]int{"AR201": 2}},
		{"new key wins over the alias", "[lint.budget]\nAR201 = 2\nAR401 = 3\n[lint.ratchet]\nAR201 = 1\n", "",
			map[string]int{"AR201": 1, "AR401": 3}},
		{"overlay accepts the new key", "[lint]\nfail_on = \"warning\"\n", "[lint.ratchet]\nAR201 = 5\n",
			map[string]int{"AR201": 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base := t.TempDir()
			dir := filepath.Join(base, ".ai-rulez")
			writeProjectFile(t, dir, "config.toml", overlayMainTOML+"\n"+tt.lint)
			if tt.local != "" {
				writeProjectFile(t, dir, "config.local.toml", tt.local)
			}

			// Act
			cfg, err := LoadConfig(context.Background(), base)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.Lint.Ratchet)
		})
	}
}

func TestLoadConfig_SwappedLintTablesAreTargeted(t *testing.T) {
	tests := []struct {
		name    string
		main    string
		local   string
		wantErr []string
	}{
		{"size table under budget", "[lint.budget.skill]\nmax_lines = 3\n", "",
			[]string{"[lint.budget.skill]", "[lint.budgets.skill]"}},
		{"size table under tolerate", "[lint.tolerate.skill]\nmax_lines = 3\n", "",
			[]string{"[lint.tolerate.skill]", "[lint.budgets.skill]"}},
		{"rule count under budgets", "[lint.budgets]\nAR201 = 1\n", "",
			[]string{"[lint.budgets] AR201 = 1", "[lint.ratchet] AR201 = 1"}},
		{"swap in the overlay", "[lint]\nfail_on = \"warning\"\n", "[lint.budgets]\nAR201 = 1\n",
			[]string{"[lint.ratchet] AR201 = 1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base := t.TempDir()
			dir := filepath.Join(base, ".ai-rulez")
			writeProjectFile(t, dir, "config.toml", overlayMainTOML+"\n"+tt.main)
			if tt.local != "" {
				writeProjectFile(t, dir, "config.local.toml", tt.local)
			}

			// Act
			_, err := LoadConfig(context.Background(), base)

			// Assert
			require.Error(t, err)
			for _, want := range tt.wantErr {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}
