package presets

import (
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestResolveAgentEffort(t *testing.T) {
	t.Parallel()

	mkAgent := func(effort string) config.ContentFile {
		if effort == "" {
			return config.ContentFile{Name: "a"}
		}
		return config.ContentFile{Name: "a", Metadata: &config.Metadata{Effort: effort}}
	}

	tests := []struct {
		name   string
		preset string
		agent  config.ContentFile
		cfg    *config.Config
		want   string
	}{
		{
			name:   "agent metadata wins over per-preset",
			preset: "claude",
			agent:  mkAgent("xhigh"),
			cfg: &config.Config{Defaults: &config.DefaultsConfig{
				Effort:         "low",
				EffortByPreset: map[string]string{"claude": "high"},
			}},
			want: "xhigh",
		},
		{
			name:   "per-preset beats global default",
			preset: "claude",
			agent:  mkAgent(""),
			cfg: &config.Config{Defaults: &config.DefaultsConfig{
				Effort:         "low",
				EffortByPreset: map[string]string{"claude": "high"},
			}},
			want: "high",
		},
		{
			name:   "fallback to global default",
			preset: "claude",
			agent:  mkAgent(""),
			cfg:    &config.Config{Defaults: &config.DefaultsConfig{Effort: "medium"}},
			want:   "medium",
		},
		{
			name:   "per-preset for different preset is ignored",
			preset: "claude",
			agent:  mkAgent(""),
			cfg: &config.Config{Defaults: &config.DefaultsConfig{
				Effort:         "low",
				EffortByPreset: map[string]string{"codex": "high"},
			}},
			want: "low",
		},
		{
			name:   "nil cfg returns empty",
			preset: "claude",
			agent:  mkAgent(""),
			cfg:    nil,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ResolveAgentEffort(tt.preset, tt.agent, tt.cfg))
		})
	}
}

func TestMapEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		preset string
		tier   string
		want   string
	}{
		// claude: pass-through
		{"claude", "low", "low"},
		{"claude", "max", "max"},
		{"claude", "inherit", "inherit"},

		// codex: max → high, inherit dropped
		{"codex", "low", "low"},
		{"codex", "xhigh", "xhigh"},
		{"codex", "max", "high"},
		{"codex", "inherit", ""},

		// amp: xhigh → high, max preserved
		{"amp", "xhigh", "high"},
		{"amp", "max", "max"},
		{"amp", "inherit", ""},

		// windsurf: max → high
		{"windsurf", "max", "high"},
		{"windsurf", "xhigh", "xhigh"},

		// antigravity: same as windsurf
		{"antigravity", "max", "high"},
		{"antigravity", "low", "low"},

		// continue-dev: caps at high
		{"continue-dev", "xhigh", "high"},
		{"continue-dev", "max", "high"},
		{"continue-dev", "low", "low"},

		// unknown preset returns ""
		{"junie", "high", ""},

		// empty tier always returns ""
		{"claude", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.preset+"/"+tt.tier, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, MapEffort(tt.preset, tt.tier))
		})
	}
}
