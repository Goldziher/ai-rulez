package config

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewConfig_Validate(t *testing.T) {
	tests := []struct {
		name string
		cfg  *ReviewConfig
		want string // substring of the expected problem; empty means valid
	}{
		{"nil", nil, ""},
		{"valid", &ReviewConfig{Content: "full", Exclude: []string{"internal-*"}, MaxCostUSD: 1, MaxCalls: 5}, ""},
		{"bad content", &ReviewConfig{Content: "everything"}, "review.content"},
		{"negative cost", &ReviewConfig{MaxCostUSD: -1}, "max_cost_usd"},
		{"NaN cost", &ReviewConfig{MaxCostUSD: math.NaN()}, "review.max_cost_usd must be a finite number"},
		{"infinite cost", &ReviewConfig{MaxCostUSD: math.Inf(1)}, "review.max_cost_usd must be a finite number"},
		{"negative infinite cost", &ReviewConfig{MaxCostUSD: math.Inf(-1)}, "review.max_cost_usd must be a finite number"},
		{"negative calls", &ReviewConfig{MaxCalls: -1}, "max_calls"},
		{"bad glob", &ReviewConfig{Exclude: []string{"[a"}}, "not a valid glob"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := strings.Join(tt.cfg.Validate(), "; ")

			// Assert
			if tt.want == "" {
				assert.Empty(t, got)
				return
			}
			assert.Contains(t, got, tt.want)
		})
	}
}

func TestLoadConfig_ReadsReviewTable(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	root := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(root, 0o755))
	body := "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n\n[review]\nrubric = \"mine\"\ncontent = \"full\"\nexclude = [\"internal-*\"]\nmax_cost_usd = 0.25\nmax_calls = 40\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.toml"), []byte(body), 0o600))

	// Act
	cfg, err := LoadConfig(context.Background(), dir)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, cfg.Review)
	assert.Equal(t, ReviewConfig{Rubric: "mine", Content: "full", Exclude: []string{"internal-*"}, MaxCostUSD: 0.25, MaxCalls: 40}, *cfg.Review)
}
