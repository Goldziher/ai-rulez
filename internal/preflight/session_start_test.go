package preflight

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestCollect_AnnouncesTheSessionStartHookForClaudeOnly(t *testing.T) {
	tests := []struct {
		name    string
		session *config.SessionStartConfig
		want    bool
	}{
		{name: "on by default", want: true},
		{name: "when always", session: &config.SessionStartConfig{When: config.SessionStartWhenAlways}, want: true},
		{name: "generate = false", session: &config.SessionStartConfig{Generate: boolPtr(false)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{BaseDir: t.TempDir(), SessionStart: tt.session}
			for _, name := range config.MCPHarnessNames() { // every harness with settings, so skipped ones would show
				cfg.Presets = append(cfg.Presets, config.Preset{Name: name})
			}
			cfg.EnableSessionStartHooks()

			items := Collect(cfg)

			var hooks []Item
			for _, it := range items {
				if it.Kind == "hook" {
					hooks = append(hooks, it)
				}
			}
			if !tt.want {
				assert.Empty(t, hooks)
				return
			}
			require.Len(t, hooks, 1)
			assert.Equal(t, []string{config.HarnessClaude}, hooks[0].Where, "no other harness is announced, because none gets the hook")
			assert.Contains(t, hooks[0].Command, "SessionStart: sh -c")
			assert.Contains(t, hooks[0].Command, "ai-rulez session-start")
		})
	}
}

func TestNewCommands_ASessionStartHookIsAnnouncedOnceAndAgainWhenItChanges(t *testing.T) {
	testutil.IsolateHome(t)
	baseDir, configDir := t.TempDir(), t.TempDir()
	build := func(version string) *config.Config {
		cfg := &config.Config{BaseDir: baseDir, ConfigDir: configDir, Presets: []config.Preset{{Name: config.HarnessClaude}},
			SessionStart: &config.SessionStartConfig{Version: version}}
		cfg.EnableSessionStartHooks()
		return cfg
	}

	first := NewCommands(build("5.4.1"), true) // a fresh clone: no previous run, everything is announced
	require.NoError(t, os.WriteFile(filepath.Join(configDir, generator.GeneratedManifestNames()[0]), []byte("{}"), 0o644))
	second := NewCommands(build("5.4.1"), true)
	changed := NewCommands(build("5.5.0"), true)

	assert.Len(t, first, 1)
	assert.Empty(t, second, "an unchanged hook is not announced again")
	assert.Len(t, changed, 1, "a changed launcher is announced")
}

func boolPtr(b bool) *bool { return &b }
