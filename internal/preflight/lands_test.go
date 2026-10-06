package preflight

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func landsConfig(t *testing.T, matcher string, allow []string) *config.Config {
	t.Helper()
	cfg := &config.Config{BaseDir: t.TempDir()}
	for _, name := range config.MCPHarnessNames() {
		cfg.Presets = append(cfg.Presets, config.Preset{Name: name})
	}
	for _, name := range []string{"hermes", "junie", "kimi", "zcode", "cline"} {
		if !slices.ContainsFunc(cfg.Presets, func(p config.Preset) bool { return p.GetName() == name }) {
			cfg.Presets = append(cfg.Presets, config.Preset{Name: name})
		}
	}
	cfg.Hooks = []config.HookGroup{{
		Event: "PreToolUse", Matcher: matcher,
		Hooks: []config.HookAction{{Command: "echo guard"}},
	}}
	if allow != nil {
		cfg.Permissions = &config.Permissions{Allow: allow}
	}
	return cfg
}

func whereOf(t *testing.T, items []Item, kind string) []string {
	t.Helper()
	for _, it := range items {
		if it.Kind == kind {
			return it.Where
		}
	}
	return nil
}

func TestCollect_HookWhereFollowsTheTranslation(t *testing.T) {
	tests := []struct {
		name    string
		matcher string
		script  string
		want    []string
		not     []string
	}{
		{
			name: "a plain command", want: []string{"claude", "amp", "cline", "kilo", "mimocode", "opencode", "pi", "poolside", "goose"},
			not: []string{"hermes", "junie", "kimi", "zcode"},
		},
		{
			name: "a matcher", matcher: "Bash", want: []string{"claude", "amp", "kilo", "mimocode", "opencode", "pi"},
			not: []string{"hermes", "junie", "kimi", "zcode", "bob", "commandcode", "reasonix"},
		},
		{
			name: "a project script", script: "hooks/guard.sh", want: []string{"claude"},
			not: []string{"hermes", "augment", "goose", "poolside"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := landsConfig(t, tt.matcher, nil)
			cfg.Presets = append(cfg.Presets, config.Preset{Name: "bob"}, config.Preset{Name: "commandcode"}, config.Preset{Name: "reasonix"})
			if tt.script != "" {
				cfg.Hooks[0].Hooks = []config.HookAction{{Script: tt.script}}
			}

			// Act
			where := whereOf(t, Collect(cfg), "hook")

			// Assert
			for _, name := range tt.want {
				assert.Contains(t, where, name, "%s writes the hook", name)
			}
			for _, name := range tt.not {
				assert.NotContains(t, where, name, "%s gets nothing from it", name)
			}
		})
	}
}

func TestCollect_AllowWhereSkipsHarnessesWithoutAnAllowList(t *testing.T) {
	// Arrange
	cfg := landsConfig(t, "", []string{"Bash(go test:*)"})

	// Act
	where := whereOf(t, Collect(cfg), "allow")

	// Assert
	require.Contains(t, where, "claude")
	for _, skipped := range []string{"cursor", "copilot-cli", "zed", "hermes", "kimi"} {
		assert.NotContains(t, where, skipped)
	}
}

func TestCollect_ProbingDoesNotChangeTheConfig(t *testing.T) {
	// Arrange
	cfg := landsConfig(t, "Bash", []string{"Bash(ls)"})

	// Act
	_ = Collect(cfg)

	// Assert
	require.Len(t, cfg.Hooks, 1)
	assert.Equal(t, []string{"Bash(ls)"}, cfg.Permissions.Allow)
}
