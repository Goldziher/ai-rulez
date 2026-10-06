package telemetry

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildStatus_ReportsConsentPendingCursorAndFailures(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	log := filepath.Join(dir, "usage.jsonl")
	spool := &Spool{Dir: dir}
	require.NoError(t, spool.PlaceCursor(log, false))
	appendLog(t, log, 0, 4)
	queued := logEvent(9)
	require.NoError(t, spool.Append(&queued))
	require.NoError(t, spool.UpdateState(func(st *State) {
		st.LastAttempt, st.LastStatus, st.LastError, st.Failures, st.ConsecutiveFailures = "2026-10-05T09:00:00Z", "retry", "collector returned 503", 3, 2
	}))
	s := Resolve(Layers{Consent: record(consentEndpoint, "http/json", false, false), Getenv: env()})

	// Act
	st := BuildStatus(&s, dir, log)

	// Assert
	assert.True(t, st.Export)
	assert.Equal(t, ConsentRecord, st.Consent.State)
	assert.Equal(t, "collector.example.org:4318", st.EndpointHost)
	assert.Equal(t, 1, st.Pending.Outbox)
	assert.Equal(t, 4, st.Pending.Log)
	assert.True(t, st.Cursor.Set)
	assert.Equal(t, int64(3), st.Delivery.Failures)
	assert.Equal(t, int64(2), st.Delivery.ConsecutiveFailures)
	assert.Contains(t, st.Withheld, "ai_rulez.session")

	var text bytes.Buffer
	st.Render(&text)
	assert.Contains(t, text.String(), "export on")
	assert.Contains(t, text.String(), "granted by the consent record")
	assert.Contains(t, text.String(), "failed flushes 3 (2 in a row)")
	assert.Contains(t, text.String(), "last error: collector returned 503")
	assert.NotContains(t, text.String(), "/v1/", "no endpoint path")

	raw, err := json.Marshal(st)
	require.NoError(t, err)
	assert.JSONEq(t, `3`, jsonField(t, raw, "delivery", "failures"))
}

func jsonField(t *testing.T, raw []byte, path ...string) string {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	var cur any = doc
	for _, key := range path {
		cur = cur.(map[string]any)[key]
	}
	out, err := json.Marshal(cur)
	require.NoError(t, err)
	return string(out)
}

func TestBuildStatus_OffAndStale(t *testing.T) {
	dir := t.TempDir()
	off := Resolve(Layers{Getenv: env()})
	st := BuildStatus(&off, dir, filepath.Join(dir, "usage.jsonl"))
	assert.False(t, st.Recording)
	assert.False(t, st.Export)
	var text bytes.Buffer
	st.Render(&text)
	assert.Contains(t, text.String(), "telemetry: off")
	assert.Contains(t, text.String(), "ai-rulez telemetry enable")

	stale := Resolve(Layers{Consent: record(consentEndpoint, "http/json", false, false), Getenv: env(EnvEndpoint, "https://other.example.org")})
	st = BuildStatus(&stale, dir, "")
	text.Reset()
	st.Render(&text)
	assert.Equal(t, ConsentStale, st.Consent.State)
	assert.Contains(t, text.String(), "stale: the endpoint changed")
}
