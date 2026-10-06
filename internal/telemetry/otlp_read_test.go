package telemetry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeNDJSON(t *testing.T, events []Event, extraLines ...string) string {
	t.Helper()
	file, err := (&Encoder{IncludeSession: true}).EncodeFile(events)
	require.NoError(t, err)
	data := file.Data
	for _, line := range extraLines {
		data = append(data, []byte(line+"\n")...)
	}
	path := filepath.Join(t.TempDir(), "usage.ndjson")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func TestReadOTLPFile_RoundTripsSkillLoadsWithTheirDigestScheme(t *testing.T) {
	// Arrange: a canonical-digest load, a served-digest load, a blake3-hash load, a rule and an eval result.
	canonical := Event{Time: "2026-10-05T09:12:41Z", EventID: "aaaaaaaaaaaaaaa1", Kind: KindSkill, ID: "deploy", Digest: lockDigest, DigestScheme: usage.DigestSchemeSkill, Source: SourceHook, Harness: "claude", Session: "5b1c0e9a7d3f2a64", Outcome: OutcomeLoaded, LoadReason: "tool"}
	served := Event{Time: "2026-10-05T09:12:42Z", EventID: "aaaaaaaaaaaaaaa2", Kind: KindSkill, ID: "deploy", Digest: lockDigest, DigestScheme: usage.DigestSchemeServed, Source: SourceMCP, Served: true, Outcome: OutcomeUsed}
	hashed := Event{Time: "2026-10-05T09:12:43Z", EventID: "aaaaaaaaaaaaaaa3", Kind: KindSkill, ID: "old", Digest: "blake3:deadbeef", Source: SourceHook, Outcome: OutcomeLoaded}
	rule := Event{Time: "2026-10-05T09:12:44Z", EventID: "aaaaaaaaaaaaaaa4", Kind: KindRule, ID: "atomic", Source: SourceHook, Outcome: OutcomeLoaded}
	evalEvents, _ := EvalEvents(sampleEvalResults())
	events := []Event{canonical, served, hashed, rule, evalEvents[0]}
	for i := range events {
		require.NoError(t, events[i].Normalize())
	}
	path := writeNDJSON(t, events, "not json", `{"resourceMetrics":[]}`)

	// Act
	read, err := ReadOTLPFile(path)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 2, read.Skipped, "a foreign line is counted, not an error")
	assert.Equal(t, 5, read.Records)
	assert.Equal(t, 2, read.Ignored, "the rule and the eval result are not skill loads")
	require.Len(t, read.Entries, 3)
	got := read.Entries
	assert.Equal(t, usage.Entry{
		Version: usage.EntrySchemaVersion, Time: "2026-10-05T09:12:41Z", Event: usage.EventSkillInvoked, Skill: "deploy", ID: "deploy",
		Session: "5b1c0e9a7d3f2a64", Invocation: "tool", Harness: "claude", Outcome: "loaded", Digest: lockDigest, DigestScheme: usage.DigestSchemeSkill, EventID: "aaaaaaaaaaaaaaa1",
	}, got[0])
	assert.Equal(t, usage.DigestSchemeServed, got[1].DigestScheme, "the scheme survives the export, so a served digest is not joined as canonical")
	assert.True(t, got[1].Served)
	assert.Equal(t, "used", got[1].Outcome)
	assert.Equal(t, "blake3:deadbeef", got[2].Hash)
	assert.Empty(t, got[2].Digest)
}

func TestReadOTLPFile_RefusesWhatTheRecorderWouldRefuse(t *testing.T) {
	line := `{"resourceLogs":[{"scopeLogs":[{"logRecords":[` +
		`{"timeUnixNano":"1790841600000000000","attributes":[{"key":"event.name","value":{"stringValue":"ai_rulez.item.loaded"}},{"key":"ai_rulez.item.kind","value":{"stringValue":"skill"}},{"key":"ai_rulez.item.id","value":{"stringValue":"../etc/passwd"}}]},` +
		`{"timeUnixNano":"1790841600000000000","attributes":[{"key":"event.name","value":{"stringValue":"ai_rulez.item.loaded"}},{"key":"ai_rulez.item.kind","value":{"stringValue":"skill"}},{"key":"ai_rulez.item.id","value":{"stringValue":"ok"}},{"key":"ai_rulez.item.digest","value":{"stringValue":"sha256:x y"}},{"key":"ai_rulez.harness","value":{"stringValue":"Bad Harness"}},{"key":"ai_rulez.outcome","value":{"stringValue":"maybe"}},{"key":"ai_rulez.event_id","value":{"stringValue":"nope"}}]}` +
		`]}]}]}`
	path := filepath.Join(t.TempDir(), "x.ndjson")
	require.NoError(t, os.WriteFile(path, []byte(line+"\n"), 0o600))

	read, err := ReadOTLPFile(path)

	require.NoError(t, err)
	require.Len(t, read.Entries, 1, "the traversal id is dropped")
	e := read.Entries[0]
	assert.Equal(t, "ok", e.ID)
	assert.Empty(t, e.Digest)
	assert.Empty(t, e.Harness)
	assert.Equal(t, "loaded", e.Outcome, "an unknown outcome falls back to the one in the event name")
	assert.Empty(t, e.EventID)
	assert.Equal(t, "2026-10-01T08:00:00Z", e.Time)
}

func TestReadOTLPFile_ReadsTheGoldenExportAndMissingFilesFail(t *testing.T) {
	read, err := ReadOTLPFile(filepath.Join("testdata", "export_file.golden.ndjson"))
	require.NoError(t, err)
	assert.NotEmpty(t, read.Entries)
	for _, e := range read.Entries {
		assert.NotEmpty(t, e.ID)
	}

	_, err = ReadOTLPFile(filepath.Join(t.TempDir(), "missing.ndjson"))
	require.Error(t, err)
}
