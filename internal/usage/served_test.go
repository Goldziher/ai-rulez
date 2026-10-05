package usage

import (
	"os"
	"path/filepath"
	"runtime"
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
