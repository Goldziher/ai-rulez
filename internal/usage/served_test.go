package usage

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordServed_WritesAnIdentifierOnlyServedEntry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	index := writeIndex(t, dir, SkillRecord{ID: "pdf-processing", Source: "s", Hash: "h-current"})
	log := filepath.Join(dir, "usage.jsonl")

	entry, err := RecordServed(ServedLoad{Skill: "pdf-processing", Digest: "sha256:abc", Session: "s1", Harness: "claude-code"},
		RecordOptions{LogPath: log, IndexPath: index, Now: fixedClock})
	require.NoError(t, err)
	assert.True(t, entry.Served)
	assert.Equal(t, InvocationMCP, entry.Invocation)
	assert.Equal(t, "h-current", entry.Hash, "the content hash is joined from the skills index")
	assert.Equal(t, "claude-code", entry.Harness)

	entries, skipped, err := ReadLog(log)
	require.NoError(t, err)
	require.Zero(t, skipped)
	require.Len(t, entries, 1)
	assert.Equal(t, EventSkillInvoked, entries[0].Event)
	assert.Equal(t, "pdf-processing", entries[0].ID)
	assert.True(t, entries[0].Served)
	assert.Equal(t, "sha256:abc", entries[0].Digest)
	assert.Equal(t, "2026-10-04T12:00:00Z", entries[0].Time)

	raw, err := os.ReadFile(log)
	require.NoError(t, err)
	for _, key := range []string{`"ts"`, `"event"`, `"skill"`, `"id"`, `"served":true`, `"digest"`, `"session"`, `"harness"`, `"invocation":"mcp"`} {
		assert.Contains(t, string(raw), key)
	}
	assert.NotContains(t, string(raw), "content", "a load is logged by identifier, never by content")
}

func TestRecordServed_IsOffUntilASinkIsConfigured(t *testing.T) {
	t.Parallel()
	entry, err := RecordServed(ServedLoad{Skill: "x"}, RecordOptions{Now: fixedClock})
	require.NoError(t, err)
	assert.Equal(t, "x", entry.Skill)
	assert.Equal(t, "mcp", entry.Harness)
}

func TestRecordServed_ResourceLoadsDoNotInflateReports(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	log := filepath.Join(dir, "usage.jsonl")
	opts := RecordOptions{LogPath: log, Now: fixedClock}
	for _, l := range []ServedLoad{
		{Skill: "a"}, {Skill: "a", Resource: true}, {Skill: "a", Resource: true}, {Skill: "b"},
	} {
		_, err := RecordServed(l, opts)
		require.NoError(t, err)
	}
	entries, _, err := ReadLog(log)
	require.NoError(t, err)
	require.Len(t, entries, 4)

	report := BuildReport(&Index{SchemaVersion: IndexSchemaVersion, Skills: []SkillRecord{{ID: "a", Hash: "h"}, {ID: "b", Hash: "h"}, {ID: "c", Hash: "h"}}}, entries, 0)
	counts := map[string]int{}
	for _, u := range report.Used {
		counts[u.ID] = u.Count
	}
	assert.Equal(t, map[string]int{"a": 1, "b": 1}, counts, "supporting files of a skill are not further uses")
	require.Len(t, report.Never, 1)
	assert.Equal(t, "c", report.Never[0].ID)
}

func TestRecordServed_SinkCommandReceivesTheLine(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the sink command uses cat")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "sink.txt")
	_, err := RecordServed(ServedLoad{Skill: "x", Session: "s"}, RecordOptions{SinkCommand: "cat > " + out, Now: fixedClock})
	require.NoError(t, err)
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"served":true`)
}

func TestRecordServed_CarriesRoleHarnessVersionAndASaltedSession(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "usage.jsonl")

	entry, err := RecordServed(ServedLoad{Skill: "refund-policy", Digest: "sha256:abc", Session: "mcp-session-1", Harness: "claude-code", Role: "billing-agent"},
		RecordOptions{LogPath: log, Now: fixedClock})
	require.NoError(t, err)
	assert.Equal(t, EntrySchemaVersion, entry.Version)
	assert.Equal(t, OutcomeLoaded, entry.Outcome)
	assert.Equal(t, "billing-agent", entry.Role)
	assert.Equal(t, "claude-code", entry.Harness)
	assert.True(t, entry.Served)
	assert.NotContains(t, entry.Session, "mcp-session-1", "the raw session id never reaches the log")
	assert.Len(t, entry.Session, 16)

	salt, err := os.ReadFile(filepath.Join(dir, "usage.salt"))
	require.NoError(t, err)
	assert.Equal(t, HashSession(strings.TrimSpace(string(salt)), "mcp-session-1"), entry.Session, "the same hash a hook-recorded load of that session gets")

	entries, skipped, err := ReadLog(log)
	require.NoError(t, err)
	require.Zero(t, skipped)
	assert.Equal(t, "billing-agent", entries[0].Role)
	assert.Equal(t, "sha256:abc", entries[0].Digest)
}

func TestReadLog_ReadsVersion1AndVersion2LinesTogether(t *testing.T) {
	log := filepath.Join(t.TempDir(), "usage.jsonl")
	lines := `{"ts":"2026-10-01T00:00:00Z","event":"skill_invoked","skill":"a","id":"a","session":"raw-id","invocation":"tool","harness":"claude"}
{"v":2,"ts":"2026-10-02T00:00:00Z","event":"skill_invoked","skill":"a","id":"a","session":"0123456789abcdef","invocation":"mcp","harness":"claude-code","outcome":"loaded","served":true,"role":"r","digest":"sha256:x"}
`
	require.NoError(t, os.WriteFile(log, []byte(lines), 0o600))
	entries, skipped, err := ReadLog(log)
	require.NoError(t, err)
	require.Zero(t, skipped)
	require.Len(t, entries, 2)
	assert.Zero(t, entries[0].Version)
	assert.False(t, entries[0].Served)
	assert.Equal(t, 2, entries[1].Version)
	assert.True(t, entries[1].Served)
	assert.Equal(t, "r", entries[1].Role)
}
