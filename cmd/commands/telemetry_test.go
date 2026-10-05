package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type telemetryEnv struct {
	root, xdg, log string
	spawns         *int
}

func setupTelemetry(t *testing.T, repoToml, userToml string) telemetryEnv {
	t.Helper()
	root, xdg := t.TempDir(), t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".ai-rulez"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "config.toml"), []byte("version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n"+repoToml), 0o600))
	if userToml != "" {
		require.NoError(t, os.MkdirAll(filepath.Join(xdg, "ai-rulez"), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(xdg, "ai-rulez", "config.toml"), []byte(userToml), 0o600))
	}
	t.Setenv("CLAUDE_PROJECT_DIR", root)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	for _, name := range []string{"AI_RULEZ_TELEMETRY", "DO_NOT_TRACK", "AI_RULEZ_TELEMETRY_ENDPOINT", "AI_RULEZ_TELEMETRY_ALLOW_NETWORK", "AI_RULEZ_USAGE_SALT"} {
		t.Setenv(name, "")
	}
	spawns := 0
	previous := telemetrySpawn
	telemetrySpawn = func() error { spawns++; return nil }
	t.Cleanup(func() {
		telemetrySpawn = previous
		telHarness, telRole, telRoot, telConfigDir, telFormat, telOutput, telJSON, reportItems = "", "", "", "", "json", "", false, false
		usageLog, usageIndex, usageHarness, usageRole, usageSalt, reportJSON, reportEvals, reportFeedback = "", "", "", "", "", false, "", ""
		configDir = ""
	})
	return telemetryEnv{root: root, xdg: xdg, log: filepath.Join(root, ".ai-rulez", "local", "usage.jsonl"), spawns: &spawns}
}

func hookInput(root, fields string) *bytes.Reader {
	return bytes.NewReader([]byte(`{"session_id":"s1","cwd":"` + root + `",` + fields + `}`))
}

func TestTelemetryHook_PrintsAndWritesTheTemplate(t *testing.T) {
	setupTelemetry(t, "", "")
	var out bytes.Buffer
	telemetryHookCmd.SetOut(&out)
	telFormat = "json"
	require.NoError(t, telemetryHookCmd.RunE(telemetryHookCmd, nil))
	assert.True(t, json.Valid(out.Bytes()))
	for _, event := range []string{"InstructionsLoaded", "SubagentStart", "SubagentStop", "UserPromptExpansion"} {
		assert.Contains(t, out.String(), event)
	}

	telFormat, telOutput = "toml", filepath.Join(t.TempDir(), "sub", "hooks.toml")
	require.NoError(t, telemetryHookCmd.RunE(telemetryHookCmd, nil))
	written, err := os.ReadFile(telOutput)
	require.NoError(t, err)
	assert.Contains(t, string(written), "[[hooks]]")
}

func TestTelemetryRecord_DisabledWritesNothing(t *testing.T) {
	env := setupTelemetry(t, "", "")
	require.NoError(t, runTelemetryRecord(hookInput(env.root, `"hook_event_name":"InstructionsLoaded","file_path":"`+env.root+`/CLAUDE.md","memory_type":"Project","load_reason":"session_start"`)))
	_, err := os.Stat(env.log)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(env.root, ".ai-rulez", "local"))
	assert.True(t, os.IsNotExist(err), "a disabled recorder creates no files, not even the salt")
}

func TestUsageRecord_ForwardsSkillLoadsToTheSpoolOnlyWhenUserScopeAllows(t *testing.T) {
	user := "[telemetry]\nenabled = true\nallow_network = true\notlp_endpoint = \"http://127.0.0.1:1\"\n"
	skillEvent := `{"hook_event_name":"PreToolUse","tool_name":"Skill","tool_input":{"skill":"git-workflow"},"session_id":"s1","cwd":"ROOT"}`

	env := setupTelemetry(t, "\n[telemetry]\nenabled = true\n", "")
	require.NoError(t, runUsageRecord(strings.NewReader(strings.ReplaceAll(skillEvent, "ROOT", env.root))))
	_, err := os.Stat(filepath.Join(env.root, ".ai-rulez", "local", telemetry.OutboxFileName))
	assert.True(t, os.IsNotExist(err), "repo config alone exports nothing")

	env = setupTelemetry(t, "", user)
	require.NoError(t, runUsageRecord(strings.NewReader(strings.ReplaceAll(skillEvent, "ROOT", env.root))))
	spool := &telemetry.Spool{Dir: filepath.Join(env.root, ".ai-rulez", "local")}
	events, _, err := spool.Pending()
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, telemetry.KindSkill, events[0].Kind)
	assert.Equal(t, "git-workflow", events[0].ID)
	assert.Equal(t, 1, *env.spawns, "a detached flush was requested, not run in process")
	log, err := os.ReadFile(env.log)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(log), "\n"), "the skill line is written once, by usage record")
}

func TestTelemetryDoctorAndFlush(t *testing.T) {
	env := setupTelemetry(t, "\n[telemetry]\nenabled = true\nallow_network = true\notlp_endpoint = \"https://evil.example.com/x\"\n", "")
	var out bytes.Buffer
	telemetryDoctorCmd.SetOut(&out)
	require.NoError(t, telemetryDoctorCmd.RunE(telemetryDoctorCmd, nil))
	assert.Contains(t, out.String(), "otlp export:         off")
	assert.Contains(t, out.String(), "ignored in repository config")
	assert.NotContains(t, out.String(), "evil.example.com")

	telJSON = true
	out.Reset()
	require.NoError(t, telemetryDoctorCmd.RunE(telemetryDoctorCmd, nil))
	assert.True(t, json.Valid(out.Bytes()))

	// Flush is an error interactively and silent in the background when export is off.
	assert.Error(t, telemetryFlushCmd.RunE(telemetryFlushCmd, nil))
	telBackground = true
	t.Cleanup(func() { telBackground = false })
	assert.NoError(t, telemetryFlushCmd.RunE(telemetryFlushCmd, nil))
	_ = env
}
