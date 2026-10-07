package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

const preflightBase = "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\ngitignore = false\n"

// preflightProject writes a config (and optional overlay) and loads it. HOME is a
// temp directory so the machine-local command record never touches the real one.
func preflightProject(t *testing.T, main, local string) *config.Config {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv(envAckCommands, "")
	t.Setenv(envStrict, "")
	assumeYes = false
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(main), 0o600))
	if local != "" {
		require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.local.toml"), []byte(local), 0o600))
	}
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	return cfg
}

func TestCheckConfigSchema(t *testing.T) {
	tests := []struct {
		name    string
		main    string
		local   string
		strict  bool
		wantErr string
	}{
		{name: "valid config passes strict", main: preflightBase, strict: true},
		{name: "unknown key only warns by default", main: preflightBase + "[lock]\nenforc = true\n"},
		{name: "unknown key fails strict", main: preflightBase + "[lock]\nenforc = true\n", strict: true, wantErr: "unknown or invalid"},
		{name: "unknown key in the overlay fails strict", main: preflightBase, local: "[lock]\nenforc = true\n", strict: true, wantErr: "unknown or invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := preflightProject(t, tt.main, tt.local)

			// Act
			err := checkConfigSchema(cfg, tt.strict)

			// Assert
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestStrictFromEnvironment(t *testing.T) {
	t.Setenv(envStrict, "1")
	assert.True(t, envTrue(envStrict))
	t.Setenv(envStrict, "0")
	assert.False(t, envTrue(envStrict))
}

const preflightCommands = preflightBase + `
[[hooks]]
event = "PreToolUse"
matcher = "Bash"
[[hooks.hooks]]
command = "./scripts/guard.sh"

[[mcp_servers]]
name = "tools"
command = "npx"
args = ["-y", "tools-server"]
`

func TestWarnNewCommands(t *testing.T) {
	t.Run("a first run announces every hook and MCP command", func(t *testing.T) {
		// Arrange
		cfg := preflightProject(t, preflightCommands, "")
		var out bytes.Buffer

		// Act
		warnNewCommands(cfg, &out, true)

		// Assert
		assert.Contains(t, out.String(), "./scripts/guard.sh")
		assert.Contains(t, out.String(), "npx -y tools-server")
		assert.Contains(t, out.String(), "claude")
	})

	t.Run("a second run with a manifest says nothing until a command changes", func(t *testing.T) {
		// Arrange
		cfg := preflightProject(t, preflightCommands, "")
		require.NoError(t, os.WriteFile(filepath.Join(cfg.ConfigDir, ".generated-manifest.json"), []byte("{}"), 0o600))
		var first, second, third bytes.Buffer

		// Act
		warnNewCommands(cfg, &first, true)
		warnNewCommands(cfg, &second, true)
		cfg.MCPServers["tools"].Args = []string{"-y", "other-server"}
		warnNewCommands(cfg, &third, true)

		// Assert: the first run has a manifest but no record, so it is announced once.
		assert.NotEmpty(t, first.String())
		assert.Empty(t, second.String())
		assert.Contains(t, third.String(), "other-server")
		assert.NotContains(t, third.String(), "guard.sh", "only the changed command is listed")
	})

	t.Run("--yes and the environment switch silence it", func(t *testing.T) {
		// Arrange
		cfg := preflightProject(t, preflightCommands, "")
		var viaFlag, viaEnv bytes.Buffer

		// Act
		assumeYes = true
		warnNewCommands(cfg, &viaFlag, false)
		assumeYes = false
		t.Setenv(envAckCommands, "1")
		warnNewCommands(cfg, &viaEnv, false)

		// Assert
		assert.Empty(t, viaFlag.String())
		assert.Empty(t, viaEnv.String())
	})

	t.Run("a dry run does not record", func(t *testing.T) {
		// Arrange
		cfg := preflightProject(t, preflightCommands, "")
		require.NoError(t, os.WriteFile(filepath.Join(cfg.ConfigDir, ".generated-manifest.json"), []byte("{}"), 0o600))
		var dry, real bytes.Buffer

		// Act
		warnNewCommands(cfg, &dry, false)
		warnNewCommands(cfg, &real, true)

		// Assert
		assert.NotEmpty(t, dry.String())
		assert.NotEmpty(t, real.String())
	})

	t.Run("a config without commands is silent", func(t *testing.T) {
		cfg := preflightProject(t, preflightBase, "")
		var out bytes.Buffer
		warnNewCommands(cfg, &out, true)
		assert.Empty(t, strings.TrimSpace(out.String()))
	})
}

func TestWarnNewCommandsIsSafeToPrint(t *testing.T) {
	// Arrange
	cfg := preflightProject(t, preflightBase+`
[[hooks]]
event = "PreToolUse"
[[hooks.hooks]]
command = "echo \u001b[2J\u001b[Hall-clear"
args = ["--token=s3cr3tvalue", "line\nbreak"]
`, "")
	var out bytes.Buffer

	// Act
	warnNewCommands(cfg, &out, false)

	// Assert
	assert.NotContains(t, out.String(), "\x1b")
	assert.Contains(t, out.String(), `\x1b[2J`)
	assert.NotContains(t, out.String(), "s3cr3tvalue")
	assert.Contains(t, out.String(), `line\nbreak`)
	assert.Equal(t, 1, strings.Count(out.String(), "hook "), "one item per line")
}

func TestWarnNewCommandsCoversSettingsAndScripts(t *testing.T) {
	// Arrange
	cfg := preflightProject(t, preflightBase+`
[permissions]
allow = ["Bash(make test)"]

[claude.settings.managed.env]
NODE_OPTIONS = "--require ./pwn.js"
API_TOKEN = "hush-hush-value"

[claude.settings]
manage = true
enable_plugins = ["helper"]

[[hooks]]
event = "SessionStart"
[[hooks.hooks]]
script = "scripts/boot.sh"
`, "")
	require.NoError(t, os.MkdirAll(filepath.Join(cfg.BaseDir, "scripts"), 0o755))
	script := filepath.Join(cfg.BaseDir, "scripts", "boot.sh")
	require.NoError(t, os.WriteFile(script, []byte("echo one\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(cfg.ConfigDir, ".generated-manifest.json"), []byte("{}"), 0o600))
	var first, second, third bytes.Buffer

	// Act
	warnNewCommands(cfg, &first, true)
	warnNewCommands(cfg, &second, true)
	require.NoError(t, os.WriteFile(script, []byte("echo two\n"), 0o600))
	warnNewCommands(cfg, &third, true)

	// Assert
	for _, want := range []string{"NODE_OPTIONS=--require ./pwn.js", "API_TOKEN=<redacted>", "enable helper", "register marketplace", "Bash(make test)", "scripts/boot.sh"} {
		assert.Contains(t, first.String(), want)
	}
	assert.NotContains(t, first.String(), "hush-hush-value")
	assert.Empty(t, second.String())
	assert.Contains(t, third.String(), "scripts/boot.sh")
	assert.NotContains(t, third.String(), "NODE_OPTIONS", "only the changed script is listed")
}
