package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelemetryRecordAndReport_EndToEnd(t *testing.T) {
	env := setupTelemetry(t, "\n[telemetry]\nenabled = true\n", "")
	manifest := `{"version":"1","files":[".claude/rules/atomic-commits.md",".claude/rules/never-read.md",".claude/agents/code-reviewer.md","CLAUDE.md"]}`
	require.NoError(t, os.WriteFile(filepath.Join(env.root, ".ai-rulez", ".generated-manifest.json"), []byte(manifest), 0o600))

	feed := func(fields string) {
		t.Helper()
		require.NoError(t, runTelemetryRecord(hookInput(env.root, fields)))
	}
	feed(`"hook_event_name":"InstructionsLoaded","file_path":"` + env.root + `/CLAUDE.md","memory_type":"Project","load_reason":"session_start"`)
	feed(`"hook_event_name":"InstructionsLoaded","file_path":"` + env.root + `/.claude/rules/atomic-commits.md","memory_type":"Project","load_reason":"path_glob_match"`)
	feed(`"hook_event_name":"SubagentStart","agent_id":"a1","agent_type":"code-reviewer"`)
	feed(`"hook_event_name":"SubagentStop","agent_id":"a1","agent_type":"code-reviewer"`)
	feed(`"hook_event_name":"Notification"`)

	info, err := os.Stat(env.log)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	// The existing skill report is unchanged by item events in the same log.
	index := filepath.Join(env.root, "idx.json")
	require.NoError(t, os.WriteFile(index, []byte(`{"schema_version":1,"skills":[]}`), 0o600))
	t.Chdir(env.root)
	usageIndex = index
	var text bytes.Buffer
	require.NoError(t, runReportUsage(&text, env.log))
	assert.Contains(t, text.String(), "Skill usage: 0 events")
	assert.NotContains(t, text.String(), "unreadable lines skipped")
	assert.Contains(t, text.String(), "Item loads: 4 events in 1 sessions")
	assert.Contains(t, text.String(), "never-read")
	assert.Contains(t, text.String(), "Rules per session: median 1.0")

	reportJSON = true
	var raw bytes.Buffer
	require.NoError(t, runReportUsage(&raw, env.log))
	var doc struct {
		Events int                    `json:"events"`
		Items  *telemetry.ItemsReport `json:"items"`
	}
	require.NoError(t, json.Unmarshal(raw.Bytes(), &doc))
	require.NotNil(t, doc.Items)
	assert.Equal(t, "never-read", doc.Items.Rules.Never[0].ID)
	assert.Equal(t, "atomic-commits", doc.Items.Rules.Loaded[0].ID)
	assert.Equal(t, 1, doc.Items.LoadReasons["rule"]["path_glob_match"])
	assert.Equal(t, "code-reviewer", doc.Items.Agents.Loaded[0].ID)
}

func TestReportUsage_WithoutItemEventsIsUnchanged(t *testing.T) {
	env := setupTelemetry(t, "", "")
	index := filepath.Join(env.root, "idx.json")
	require.NoError(t, os.WriteFile(index, []byte(`{"schema_version":1,"skills":[{"id":"a","kind":"skill","source":"s","hash":"h","outputs":{}}]}`), 0o600))
	logPath := filepath.Join(env.root, "u.jsonl")
	require.NoError(t, os.WriteFile(logPath, []byte(`{"v":2,"ts":"2026-10-05T09:00:00Z","event":"skill_invoked","skill":"a","id":"a","invocation":"tool","harness":"claude"}`+"\n"), 0o600))
	usageIndex, reportJSON = index, true
	var raw bytes.Buffer
	require.NoError(t, runReportUsage(&raw, logPath))
	assert.NotContains(t, raw.String(), `"items"`, "the JSON shape only grows when item events exist")

	reportItems = true
	raw.Reset()
	require.NoError(t, runReportUsage(&raw, logPath))
	assert.Contains(t, raw.String(), `"items"`, "--items forces the section")
}
