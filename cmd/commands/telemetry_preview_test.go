package commands

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetPreviewFlags(t *testing.T) {
	t.Helper()
	telLog, telLimit, telWithEvals = "", 5, false
	t.Cleanup(func() { telLog, telLimit, telWithEvals = "", 5, false })
}

func recordSkillLoads(t *testing.T, env telemetryEnv, skills ...string) {
	t.Helper()
	for _, skill := range skills {
		event := `{"hook_event_name":"PreToolUse","tool_name":"Skill","tool_input":{"skill":"` + skill + `"},"session_id":"s1","cwd":"` + jsonPath(env.root) + `"}`
		require.NoError(t, runUsageRecord(strings.NewReader(event)))
	}
}

func TestTelemetryPreview_ShowsWhatWouldBeSentWithoutExportOn(t *testing.T) {
	resetPreviewFlags(t)
	env := setupTelemetry(t, "", "")
	recordSkillLoads(t, env, "deploy", "release-notes")
	var out bytes.Buffer

	require.NoError(t, runTelemetryPreview(&out))

	text := out.String()
	assert.Contains(t, text, "usage log")
	assert.Contains(t, text, "2 events, previewing 2")
	assert.Contains(t, text, "export: off")
	assert.Contains(t, text, "POST <no endpoint configured>/v1/logs  (2 events, gzip,")
	assert.Contains(t, text, `"stringValue":"deploy"`)
	assert.Contains(t, text, `"stringValue":"release-notes"`)
	assert.Contains(t, text, "fields exported: event.name, ai_rulez.item.kind")
	assert.Contains(t, text, "fields withheld: ai_rulez.item.path, ai_rulez.session")
	assert.Contains(t, text, "timestamps: the observation and metric times")
	assert.Contains(t, text, "Nothing was sent.")
	assert.NotContains(t, text, "5b1c", "a session hash is withheld by default")
}

func TestTelemetryPreview_LimitAndEmptyLog(t *testing.T) {
	resetPreviewFlags(t)
	env := setupTelemetry(t, "", "")
	var out bytes.Buffer
	require.NoError(t, runTelemetryPreview(&out))
	assert.Contains(t, out.String(), "(missing)")
	assert.Contains(t, out.String(), "Nothing to send.")

	recordSkillLoads(t, env, "a", "b", "c")
	telLimit = 2
	out.Reset()
	require.NoError(t, runTelemetryPreview(&out))
	assert.Contains(t, out.String(), "3 events, previewing 2")
	assert.NotContains(t, out.String(), `"stringValue":"c"`)

	telLimit = 0
	out.Reset()
	require.NoError(t, runTelemetryPreview(&out))
	assert.Contains(t, out.String(), "3 events, previewing 3")

	telLimit = -1
	assert.Error(t, runTelemetryPreview(&out))
}

func TestTelemetryPreview_NamedLogMustExist(t *testing.T) {
	resetPreviewFlags(t)
	setupTelemetry(t, "", "")
	telLog = filepath.Join(t.TempDir(), "missing.jsonl")

	assert.Error(t, runTelemetryPreview(&bytes.Buffer{}))
}

func TestTelemetryPreview_ReadsTheOutboxWhenExportIsActiveAndMakesNoRequest(t *testing.T) {
	resetPreviewFlags(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer server.Close()
	user := "[telemetry]\nenabled = true\nallow_network = true\notlp_endpoint = \"" + server.URL + "/otlp\"\n"
	env := setupTelemetry(t, "", user)
	recordSkillLoads(t, env, "deploy")
	var out bytes.Buffer

	require.NoError(t, runTelemetryPreview(&out))

	text := out.String()
	assert.Contains(t, text, "source: outbox .ai-rulez/local/"+telemetry.OutboxFileName)
	assert.Contains(t, text, "export: on")
	assert.Contains(t, text, "POST "+server.URL+"/otlp/v1/logs")
	assert.Zero(t, hits.Load(), "a preview opens no connection")
	pending, _, err := (&telemetry.Spool{Dir: filepath.Join(env.root, ".ai-rulez", "local")}).Pending()
	require.NoError(t, err)
	assert.Len(t, pending, 1, "a preview leaves the outbox untouched")
}

func TestUsageRecord_LogLineAndOutboxShareTheEventID(t *testing.T) {
	user := "[telemetry]\nenabled = true\nallow_network = true\notlp_endpoint = \"http://127.0.0.1:1\"\n"
	env := setupTelemetry(t, "", user)
	recordSkillLoads(t, env, "deploy")

	data, err := os.ReadFile(env.log)
	require.NoError(t, err)
	read, err := telemetry.ReadLogEvents(env.log)
	require.NoError(t, err)
	pending, _, err := (&telemetry.Spool{Dir: filepath.Join(env.root, ".ai-rulez", "local")}).Pending()
	require.NoError(t, err)

	require.Len(t, read.Events, 1)
	require.Len(t, pending, 1)
	assert.Contains(t, string(data), `"event_id":"`+pending[0].EventID+`"`)
	assert.Equal(t, pending[0].EventID, read.Events[0].EventID)
}

func TestTelemetryPreview_WithEvalsShowsResultsAndGauges(t *testing.T) {
	resetPreviewFlags(t)
	env := setupTelemetry(t, "", "")
	t.Chdir(env.root)
	recordSkillLoads(t, env, "deploy")
	writeStore(t, env.root)
	telWithEvals, telLimit = true, 0
	var out bytes.Buffer

	require.NoError(t, runTelemetryPreview(&out))

	text := out.String()
	assert.Contains(t, text, "ai_rulez.eval.result")
	assert.Contains(t, text, "ai_rulez.skill.eval.pass_rate")
	assert.Contains(t, text, "ai_rulez.eval.pass_rate")
	assert.Contains(t, text, "Nothing was sent.")
}
