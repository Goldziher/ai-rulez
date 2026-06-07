package presets

import (
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestResolveAgentModel(t *testing.T) {
	t.Parallel()

	mkAgent := func(extra map[string]string) config.ContentFile {
		if extra == nil {
			return config.ContentFile{Name: "a"}
		}
		return config.ContentFile{Name: "a", Metadata: &config.Metadata{Extra: extra}}
	}

	tests := []struct {
		name   string
		preset string
		agent  config.ContentFile
		cfg    *config.Config
		want   string
	}{
		{
			name:   "no metadata, no defaults returns empty",
			preset: "claude",
			agent:  mkAgent(nil),
			cfg:    nil,
			want:   "",
		},
		{
			name:   "legacy model only, no defaults",
			preset: "claude",
			agent:  mkAgent(map[string]string{"model": "sonnet"}),
			cfg:    nil,
			want:   "sonnet",
		},
		{
			name:   "preset-scoped model wins over legacy model",
			preset: "claude",
			agent:  mkAgent(map[string]string{"model": "sonnet", "claude_model": "opus"}),
			cfg:    nil,
			want:   "opus",
		},
		{
			name:   "preset-scoped model wins over defaults map",
			preset: "claude",
			agent:  mkAgent(map[string]string{"claude_model": "opus"}),
			cfg: &config.Config{Defaults: &config.DefaultsConfig{
				ModelByPreset: map[string]string{"claude": "haiku"},
			}},
			want: "opus",
		},
		{
			name:   "defaults map wins over legacy model",
			preset: "claude",
			agent:  mkAgent(map[string]string{"model": "sonnet"}),
			cfg: &config.Config{Defaults: &config.DefaultsConfig{
				ModelByPreset: map[string]string{"claude": "haiku"},
			}},
			want: "haiku",
		},
		{
			name:   "defaults map for different preset is ignored, legacy used",
			preset: "claude",
			agent:  mkAgent(map[string]string{"model": "sonnet"}),
			cfg: &config.Config{Defaults: &config.DefaultsConfig{
				ModelByPreset: map[string]string{"copilot": "gpt-5"},
			}},
			want: "sonnet",
		},
		{
			name:   "preset-scoped model for different preset is ignored",
			preset: "claude",
			agent:  mkAgent(map[string]string{"copilot_model": "gpt-5", "model": "sonnet"}),
			cfg:    nil,
			want:   "sonnet",
		},
		{
			name:   "defaults-only model applies when agent has no model",
			preset: "claude",
			agent:  mkAgent(nil),
			cfg: &config.Config{Defaults: &config.DefaultsConfig{
				ModelByPreset: map[string]string{"claude": "haiku"},
			}},
			want: "haiku",
		},
		{
			name:   "empty preset-scoped value falls through",
			preset: "claude",
			agent:  mkAgent(map[string]string{"claude_model": "", "model": "sonnet"}),
			cfg:    nil,
			want:   "sonnet",
		},
		{
			name:   "nil defaults config tolerated",
			preset: "claude",
			agent:  mkAgent(map[string]string{"model": "sonnet"}),
			cfg:    &config.Config{},
			want:   "sonnet",
		},
		{
			name:   "hyphenated preset name resolves",
			preset: "continue-dev",
			agent:  mkAgent(map[string]string{"continue-dev_model": "mistral-large"}),
			cfg:    nil,
			want:   "mistral-large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ResolveAgentModel(tt.preset, tt.agent, tt.cfg))
		})
	}
}

func TestResolveGlobalModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		preset string
		cfg    *config.Config
		want   string
	}{
		{name: "nil cfg", preset: "claude", cfg: nil, want: ""},
		{name: "nil defaults", preset: "claude", cfg: &config.Config{}, want: ""},
		{
			name:   "preset hit",
			preset: "claude",
			cfg: &config.Config{Defaults: &config.DefaultsConfig{
				ModelByPreset: map[string]string{"claude": "opus"},
			}},
			want: "opus",
		},
		{
			name:   "preset miss",
			preset: "copilot",
			cfg: &config.Config{Defaults: &config.DefaultsConfig{
				ModelByPreset: map[string]string{"claude": "opus"},
			}},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ResolveGlobalModel(tt.preset, tt.cfg))
		})
	}
}

// TestAgentModelOverride_PerPreset checks each agent-emitting preset routes its
// own <preset>_model key into the rendered frontmatter and ignores other presets' keys.
func TestAgentModelOverride_PerPreset(t *testing.T) {
	t.Parallel()

	agent := config.ContentFile{
		Name: "researcher",
		Metadata: &config.Metadata{
			Extra: map[string]string{
				"model":              "legacy-shared",
				"claude_model":       "opus",
				"copilot_model":      "gpt-5",
				"cursor_model":       "cursor-fast",
				"cline_model":        "cline-anthropic",
				"opencode_model":     "opencode-large",
				"windsurf_model":     "windsurf-pro",
				"continue-dev_model": "continue-pro",
				"gemini_model":       "gemini-2.0",
				"description":        "Research helper",
			},
		},
	}

	tests := []struct {
		preset string
		got    func() map[string]interface{}
		want   string
	}{
		{
			preset: "claude",
			got:    func() map[string]interface{} { return (&ClaudePresetGenerator{}).buildAgentFrontmatter(agent, nil) },
			want:   "opus",
		},
		{
			preset: "copilot",
			got: func() map[string]interface{} {
				return (&CopilotPresetGenerator{}).buildCopilotAgentFrontmatter(agent, nil)
			},
			want: "gpt-5",
		},
		{
			preset: "cursor",
			got: func() map[string]interface{} {
				return (&CursorPresetGenerator{}).buildCursorAgentFrontmatter(agent, nil)
			},
			want: "cursor-fast",
		},
		{
			preset: "cline",
			got:    func() map[string]interface{} { return (&ClinePresetGenerator{}).buildClineAgentFrontmatter(agent, nil) },
			want:   "cline-anthropic",
		},
		{
			preset: "opencode",
			got: func() map[string]interface{} {
				return (&OpencodePresetGenerator{}).buildOpencodeAgentFrontmatter(agent, nil)
			},
			want: "opencode-large",
		},
		{
			preset: "windsurf",
			got: func() map[string]interface{} {
				return (&WindsurfPresetGenerator{}).buildWindsurfAgentFrontmatter(agent, nil)
			},
			want: "windsurf-pro",
		},
		{
			preset: "continue-dev",
			got: func() map[string]interface{} {
				return (&ContinueDevPresetGenerator{}).buildContinueDevAgentFrontmatter(agent, nil)
			},
			want: "continue-pro",
		},
		{
			preset: "gemini",
			got: func() map[string]interface{} {
				return (&GeminiPresetGenerator{}).buildGeminiAgentFrontmatter(agent, nil)
			},
			want: "gemini-2.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			t.Parallel()
			fm := tt.got()
			assert.Equal(t, tt.want, fm["model"], "preset %s should pick its own <preset>_model key", tt.preset)
		})
	}
}

// TestAgentModelOverride_DefaultsAppliesWithoutFrontmatter checks an agent with no
// frontmatter still emits a per-preset default from defaults.model_by_preset.
func TestAgentModelOverride_DefaultsAppliesWithoutFrontmatter(t *testing.T) {
	t.Parallel()

	agent := config.ContentFile{Name: "bare"}
	cfg := &config.Config{Defaults: &config.DefaultsConfig{
		ModelByPreset: map[string]string{
			"claude":  "haiku",
			"copilot": "gpt-4-turbo",
		},
	}}

	claudeFM := (&ClaudePresetGenerator{}).buildAgentFrontmatter(agent, cfg)
	assert.Equal(t, "haiku", claudeFM["model"], "claude default should apply even without agent frontmatter")

	copilotFM := (&CopilotPresetGenerator{}).buildCopilotAgentFrontmatter(agent, cfg)
	assert.Equal(t, "gpt-4-turbo", copilotFM["model"], "copilot default should apply even without agent frontmatter")
}
