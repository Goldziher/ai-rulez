package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuardCommand(t *testing.T) {
	tests := []struct {
		name          string
		guard         *GuardConfig
		binaryVersion string
		want          string
	}{
		{name: "binary version through npx", guard: &GuardConfig{Generated: true}, binaryVersion: "5.1.0", want: "npx -y ai-rulez@5.1.0 guard"},
		{name: "dev build resolves to latest", guard: &GuardConfig{Generated: true}, binaryVersion: "dev", want: "npx -y ai-rulez@latest guard"},
		{name: "configured version wins", guard: &GuardConfig{Generated: true, Version: "4.0.0"}, binaryVersion: "5.1.0", want: "npx -y ai-rulez@4.0.0 guard"},
		{name: "command replaces npx", guard: &GuardConfig{Generated: true, Command: []string{"ai-rulez"}}, binaryVersion: "5.1.0", want: "ai-rulez guard"},
		{name: "command words are quoted", guard: &GuardConfig{Generated: true, Command: []string{"/opt/my tools/ai-rulez"}}, want: "'/opt/my tools/ai-rulez' guard"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Guard: tt.guard}

			assert.Equal(t, tt.want, cfg.GuardCommand(tt.binaryVersion))
		})
	}
}

func TestEnableGuardHooks(t *testing.T) {
	t.Run("adds one group, once", func(t *testing.T) {
		cfg := &Config{Guard: &GuardConfig{Generated: true}, Hooks: []HookGroup{{Event: "Stop"}}}

		cfg.EnableGuardHooks("5.0.0")
		cfg.EnableGuardHooks("5.0.0")

		require.Len(t, cfg.Hooks, 2)
		g := cfg.Hooks[1]
		assert.Equal(t, "PreToolUse", g.Event)
		assert.Equal(t, HookBuiltinGuard, g.Builtin)
		assert.ElementsMatch(t, GuardHarnesses, g.Targets)
		assert.Equal(t, "npx -y ai-rulez@5.0.0 guard", g.Hooks[0].Command)
		assert.Len(t, cfg.UserHooks(), 1, "the synthesized group is not a user hook")
	})
	for name, cfg := range map[string]*Config{
		"off":        {Guard: &GuardConfig{}},
		"absent":     {},
		"user scope": {Guard: &GuardConfig{Generated: true}, UserScope: true},
	} {
		t.Run("does nothing when "+name, func(t *testing.T) {
			cfg.EnableGuardHooks("5.0.0")

			assert.Empty(t, cfg.Hooks)
		})
	}
}

func TestConfigValidateGuard(t *testing.T) {
	tests := []struct {
		name    string
		guard   *GuardConfig
		wantErr string
	}{
		{name: "absent"},
		{name: "enabled", guard: &GuardConfig{Generated: true}},
		{name: "version pin", guard: &GuardConfig{Generated: true, Version: "5.0.0"}},
		{name: "command", guard: &GuardConfig{Generated: true, Command: []string{"ai-rulez"}}},
		{name: "version without generated", guard: &GuardConfig{Version: "5.0.0"}, wantErr: "require guard.generated = true"},
		{name: "command without generated", guard: &GuardConfig{Command: []string{"x"}}, wantErr: "require guard.generated = true"},
		{name: "empty executable", guard: &GuardConfig{Generated: true, Command: []string{" "}}, wantErr: "executable must not be empty"},
		{name: "version and command", guard: &GuardConfig{Generated: true, Version: "1.0.0", Command: []string{"x"}}, wantErr: "mutually exclusive"},
		{name: "shell metacharacters in version", guard: &GuardConfig{Generated: true, Version: "1;rm"}, wantErr: "not a valid version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Version: "4.0", Name: "test", Presets: []Preset{{BuiltIn: "claude"}}, Guard: tt.guard}

			err := cfg.Validate()

			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLoadConfigTOML_GuardSection(t *testing.T) {
	baseDir := writeTOMLProject(t, `version = "4.0"
name = "proj"
presets = ["claude"]

[guard]
generated = true

[[hooks]]
event = "Stop"
[[hooks.hooks]]
command = "echo done"
`)
	cfg, err := LoadConfig(context.Background(), baseDir)
	require.NoError(t, err)
	require.NotNil(t, cfg.Guard)
	assert.True(t, cfg.HasGuard())
	assert.Len(t, cfg.Hooks, 1, "loading does not add the synthesized group")

	cfg.EnableGuardHooks("5.0.0")
	out, err := MarshalTOML(cfg)

	require.NoError(t, err)
	assert.Contains(t, string(out), "generated = true")
	assert.NotContains(t, string(out), "ai-rulez@", "the synthesized hook is never written back")
	assert.Contains(t, string(out), "echo done")
}
