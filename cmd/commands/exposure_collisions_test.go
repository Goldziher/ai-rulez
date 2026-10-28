package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/exposure"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

func TestCollisionMinScore(t *testing.T) {
	tests := []struct {
		name    string
		flag    float64
		changed bool
		exp     *config.ExposureConfig
		want    float64
		wantErr string
	}{
		{name: "the default without configuration", flag: exposure.DefaultCollisionMinScore, want: exposure.DefaultCollisionMinScore},
		{name: "the configured threshold replaces the default", flag: exposure.DefaultCollisionMinScore, exp: &config.ExposureConfig{CollisionMinScore: 0.3}, want: 0.3},
		{name: "a zero threshold is off and keeps the default", flag: exposure.DefaultCollisionMinScore, exp: &config.ExposureConfig{}, want: exposure.DefaultCollisionMinScore},
		{name: "the flag wins over the configuration", flag: 0.05, changed: true, exp: &config.ExposureConfig{CollisionMinScore: 0.3}, want: 0.05},
		{name: "a zero flag compares every pair", flag: 0, changed: true, want: 0},
		{name: "above one is rejected", flag: 1.5, changed: true, wantErr: "--min-score must be between 0 and 1"},
		{name: "below zero is rejected", flag: -0.1, changed: true, wantErr: "--min-score must be between 0 and 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := collisionMinScore(tt.flag, tt.changed, tt.exp)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, tt.want, got, 0)
		})
	}
}

func TestWriteCollisionsText(t *testing.T) {
	collision := exposure.Collision{
		Score: 0.25, Shared: []string{"a1x", "b2x", "c3x", "d4x", "e5x", "f6x", "g7x", "h8x", "i9x"},
		First: ".claude/skills/charge/SKILL.md", FirstKind: exposure.KindSkill, FirstName: "charge", FirstArea: "billing",
		Second: ".claude/agents/ranker.md", SecondKind: exposure.KindAgent, SecondName: "ranker", SecondArea: "search",
	}
	tests := []struct {
		name   string
		rep    exposure.CollisionReport
		want   []string
		absent []string
	}{
		{
			name: "rows with the first eight shared words",
			rep:  exposure.CollisionReport{Root: "/r", Plugins: exposure.PluginsNone, MinScore: 0.1, ArtifactsCompared: 3, Total: 2, Collisions: []exposure.Collision{collision}},
			want: []string{
				"Collisions of /r (plugins none, min score 0.10): 1 of 2 pairs among 3 listings",
				"SCORE", "0.25", "skill charge", "billing", "agent ranker", "search", "a1x, b2x, c3x, d4x, e5x, f6x, g7x, h8x (+1)",
			},
			absent: []string{"i9x"},
		},
		{
			name:   "no pair",
			rep:    exposure.CollisionReport{Root: "/r", Plugins: exposure.PluginsEnabled, MinScore: 0.5, ArtifactsCompared: 1, Collisions: []exposure.Collision{}},
			want:   []string{"0 of 0 pairs among 1 listing", "no collisions"},
			absent: []string{"SCORE"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			require.NoError(t, writeCollisionsText(&out, &tt.rep))

			for _, w := range tt.want {
				assert.Contains(t, out.String(), w)
			}
			for _, w := range tt.absent {
				assert.NotContains(t, out.String(), w)
			}
		})
	}
}

func TestExposureLintSource(t *testing.T) {
	root := t.TempDir()
	for rel, body := range map[string]string{
		".claude/skills/charge/SKILL.md": "---\nname: charge\ndescription: Look up customer invoice details by name.\n---\nbody\n",
		".claude/skills/rank/SKILL.md":   "---\nname: rank\ndescription: Look up customer ranker details by name.\n---\nbody\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
	billing := config.ExposureArea{Name: "billing", Terms: []string{"invoice"}}
	search := config.ExposureArea{Name: "search", Terms: []string{"ranker"}}
	tests := []struct {
		name string
		exp  *config.ExposureConfig
		want []lint.ExposureCollision
	}{
		{
			name: "the pairs of the classified inventory",
			exp:  &config.ExposureConfig{Plugins: config.ExposurePluginsNone, Areas: []config.ExposureArea{billing, search}},
			want: []lint.ExposureCollision{{
				Score: 0.5, Shared: []string{"customer", "details", "look", "name"},
				First: ".claude/skills/charge/SKILL.md", FirstKind: "skill", FirstName: "charge", FirstArea: "billing",
				Second: ".claude/skills/rank/SKILL.md", SecondKind: "skill", SecondName: "rank", SecondArea: "search",
			}},
		},
		{name: "none for a map with problems", exp: &config.ExposureConfig{Areas: []config.ExposureArea{billing, search, search}}, want: nil},
		{name: "none without areas", exp: &config.ExposureConfig{}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := exposureLintSource{ctx: t.Context(), cfg: &config.Config{BaseDir: root, Exposure: tt.exp}}

			assert.Equal(t, tt.want, src.Collisions(0.1))
		})
	}
}

func TestCollisionDefaultsAgree(t *testing.T) {
	assert.InDelta(t, lint.DefaultExposureCollisionMinScore, exposure.DefaultCollisionMinScore, 0)
	assert.Equal(t, lint.DefaultExposureCollisionLimit, exposure.DefaultCollisionLimit)
}
