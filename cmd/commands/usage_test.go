package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetUsageFlags(t *testing.T) {
	t.Helper()
	usageLog, usageSinkCommand, usageIndex, telExecutable, telOutput, reportJSON = "", "", "", "ai-rulez", "", false
	previousConfigDir := configDir
	t.Cleanup(func() {
		usageLog, usageSinkCommand, usageIndex, telExecutable, telOutput, reportJSON = "", "", "", "ai-rulez", "", false
		configDir = previousConfigDir
	})
}

func TestUsageHook_PrintsAndWritesTheTemplate(t *testing.T) {
	resetUsageFlags(t)

	var out bytes.Buffer
	telemetryHookCmd.SetOut(&out)
	require.NoError(t, telemetryHookCmd.RunE(telemetryHookCmd, nil))
	assert.True(t, json.Valid(out.Bytes()))
	assert.Contains(t, out.String(), "ai-rulez usage record")

	telOutput = filepath.Join(t.TempDir(), "sub", "hooks.json")
	require.NoError(t, telemetryHookCmd.RunE(telemetryHookCmd, nil))
	written, err := os.ReadFile(telOutput)
	require.NoError(t, err)
	assert.Contains(t, string(written), "UserPromptExpansion")
}

func TestUsageRecordAndReport_EndToEnd(t *testing.T) {
	resetUsageFlags(t)
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "idx", usage.IndexFileName)
	data, err := (&usage.Index{SchemaVersion: usage.IndexSchemaVersion, Skills: []usage.SkillRecord{
		{ID: "alpha", Hash: "blake3:aa", Owner: "team"},
		{ID: "idle", Hash: "blake3:bb"},
	}}).Marshal()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(indexPath), 0o750))
	require.NoError(t, os.WriteFile(indexPath, data, 0o600))

	usageLog = filepath.Join(dir, "usage.jsonl")
	usageIndex = indexPath
	event := `{"hook_event_name":"PreToolUse","tool_name":"Skill","tool_input":{"skill":"alpha"}}`
	require.NoError(t, runUsageRecord(strings.NewReader(event)))
	require.NoError(t, runUsageRecord(strings.NewReader(`{"hook_event_name":"Stop"}`)), "other events are ignored")

	var out bytes.Buffer
	require.NoError(t, runReportUsage(&out, usageLog))
	report := out.String()
	assert.Contains(t, report, "Skill usage: 1 events")
	assert.Contains(t, report, "Never used (1)")
	assert.Contains(t, report, "idle")

	reportJSON = true
	out.Reset()
	require.NoError(t, runReportUsage(&out, usageLog))
	var decoded usage.Report
	require.NoError(t, json.Unmarshal(out.Bytes(), &decoded))
	require.Len(t, decoded.Never, 1)
	assert.Equal(t, "idle", decoded.Never[0].ID)
	assert.Empty(t, decoded.Changed)
}

func TestReportUsage_MissingIndexExplainsHowToCreateIt(t *testing.T) {
	resetUsageFlags(t)
	usageIndex = filepath.Join(t.TempDir(), "nope.json")

	err := runReportUsage(&bytes.Buffer{}, filepath.Join(t.TempDir(), "log"))
	require.Error(t, err)
}
