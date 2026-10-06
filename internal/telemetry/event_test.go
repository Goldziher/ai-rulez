package telemetry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventJSONGolden(t *testing.T) {
	e := sampleEvents()[0]
	data, err := json.MarshalIndent(e, "", "  ")
	require.NoError(t, err)
	golden(t, "event.golden.json", append(data, '\n'))
}

func TestNormalize_RejectsBadIdentityAndDropsBadOptionals(t *testing.T) {
	good := Event{Kind: KindRule, ID: "atomic-commits", Source: SourceHook, Outcome: OutcomeLoaded}
	require.NoError(t, (&Event{Kind: good.Kind, ID: good.ID, Source: good.Source, Outcome: good.Outcome}).Normalize())

	for name, mutate := range map[string]func(*Event){
		"kind":    func(e *Event) { e.Kind = "prompt" },
		"id":      func(e *Event) { e.ID = "has space" },
		"dotdot":  func(e *Event) { e.ID = "a/../b" },
		"empty":   func(e *Event) { e.ID = "" },
		"source":  func(e *Event) { e.Source = "network" },
		"outcome": func(e *Event) { e.Outcome = "maybe" },
	} {
		e := good
		mutate(&e)
		assert.Error(t, e.Normalize(), name)
	}

	e := good
	e.Path, e.Harness, e.Role, e.Session, e.Digest, e.LoadReason, e.MemoryType = "/etc/passwd", "Bad Harness", "Role!", "not-hex", "has space", "Bad-Reason", "Nope"
	require.NoError(t, e.Normalize())
	assert.Empty(t, e.Path)
	assert.Empty(t, e.Harness)
	assert.Empty(t, e.Role)
	assert.Empty(t, e.Session)
	assert.Empty(t, e.Digest)
	assert.Empty(t, e.LoadReason)
	assert.Empty(t, e.MemoryType)
}

func TestCleanRelPath(t *testing.T) {
	for _, bad := range []string{"", "/abs/path", "../up", "a/../../b", `C:\x`, "C:/x", "a\\b", ".."} {
		_, ok := cleanRelPath(bad)
		assert.False(t, ok, bad)
	}
	got, ok := cleanRelPath(".claude/rules/./x.md")
	assert.True(t, ok)
	assert.Equal(t, ".claude/rules/x.md", got)
}

func TestRecorder_StampsTimeAndIDOnce(t *testing.T) {
	var got []Event
	rec := &Recorder{Emitter: emitterFunc(func(e *Event) error { got = append(got, *e); return nil }), Clock: fixedClock, Salt: "salt"}
	e := Event{Kind: KindRule, ID: "r", Source: SourceHook, Outcome: OutcomeLoaded}
	require.NoError(t, rec.Record(context.Background(), e))
	require.NoError(t, rec.Record(context.Background(), e))
	require.Len(t, got, 2)
	assert.Equal(t, "2026-10-05T09:12:44Z", got[0].Time)
	assert.Equal(t, 1, got[0].Version)
	assert.Equal(t, EventItem, got[0].Name)
	assert.Len(t, got[0].EventID, 16)
	assert.NotEqual(t, got[0].EventID, got[1].EventID, "two loads in one second must not share an id")

	// A caller-supplied time and id are kept (replay of a spooled line).
	e.Time, e.EventID = "2026-01-01T00:00:00Z", "cafecafecafecafe"
	require.NoError(t, rec.Record(context.Background(), e))
	assert.Equal(t, "2026-01-01T00:00:00Z", got[2].Time)
	assert.Equal(t, "cafecafecafecafe", got[2].EventID)
}

type emitterFunc func(*Event) error

func (f emitterFunc) Emit(_ context.Context, e *Event) error { return f(e) }
func (emitterFunc) Close(context.Context) error              { return nil }

func TestEventFieldsAreAllowlistedOrMapped(t *testing.T) {
	// Every JSON field of Event must be exported through the allowlist or be a
	// known envelope field: a new field cannot leave the machine by accident.
	data, err := json.Marshal(Event{Version: 1, Name: "n", Time: "t", EventID: "e", Kind: "k", ID: "i", Path: "p", Digest: "d", Source: "s", Harness: "h", Role: "r", Served: true, Session: "s", Outcome: "o", LoadReason: "l", MemoryType: "m", DurationMS: 1})
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(data, &fields))
	known := map[string]bool{}
	for _, a := range Allowlist {
		known[a.Field] = true
	}
	for _, f := range Mapped {
		known[f] = true
	}
	for field := range fields {
		assert.True(t, known[field], "event field %q is neither in the export allowlist nor mapped to the envelope", field)
	}
	for _, a := range Allowlist {
		assert.Contains(t, fields, a.Field, "allowlist row %q names no event field", a.Name)
	}
}

func TestJSONL_SkillEventsStayReadableByUsageReaders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local", "usage.jsonl")
	j := JSONL{Path: path}
	events := sampleEvents()
	for i := range events {
		require.NoError(t, j.Emit(context.Background(), &events[i]))
	}
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	entries, skipped, err := usage.ReadLog(path)
	require.NoError(t, err)
	assert.Zero(t, skipped, "item events must not count as unreadable lines")
	require.Len(t, entries, 1)
	assert.Equal(t, "deploy-staging", entries[0].ID)
	assert.Equal(t, usage.EntrySchemaVersion, entries[0].Version)
	assert.True(t, entries[0].Served)

	items, err := ReadItemEvents(path)
	require.NoError(t, err)
	assert.Len(t, items, 3)
}

func TestMulti_FansOutAndJoinsErrors(t *testing.T) {
	var a, b int
	failing := emitterFunc(func(*Event) error { a++; return assert.AnError })
	counting := emitterFunc(func(*Event) error { b++; return nil })
	m := Compose(failing, nil, counting)
	e := sampleEvents()[0]
	err := m.Emit(context.Background(), &e)
	require.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, 1, a)
	assert.Equal(t, 1, b, "a failing emitter must not stop the others")
	assert.IsType(t, Nop{}, Compose())
	assert.NoError(t, m.Close(context.Background()))
}

func TestFromUsageEntry_KeepsTheLogLineEventID(t *testing.T) {
	tests := []struct {
		name    string
		eventID string
		want    string
	}{
		{"a v3 line keeps its id so log and spool dedupe", "0123456789abcdef", "0123456789abcdef"},
		{"a v2 line has none", "", ""},
		{"a malformed id is dropped", "not-an-id\nx", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := FromUsageEntry(&usage.Entry{ID: "a", Skill: "a", EventID: tt.eventID, Harness: "claude"})

			assert.Equal(t, tt.want, event.EventID)
		})
	}
}

func TestToUsageEntry_CarriesEventIDAndNamesTheServedDigestScheme(t *testing.T) {
	served := Event{Kind: KindSkill, ID: "a", Source: SourceMCP, Served: true, Digest: "sha256:abc", EventID: "0123456789abcdef"}
	hashed := Event{Kind: KindSkill, ID: "a", Source: SourceHook, Digest: "blake3:aa"}

	servedEntry, hashedEntry := ToUsageEntry(&served), ToUsageEntry(&hashed)

	assert.Equal(t, "0123456789abcdef", servedEntry.EventID)
	assert.Equal(t, "sha256:abc", servedEntry.Digest)
	assert.Equal(t, usage.DigestSchemeServed, servedEntry.DigestScheme)
	assert.Equal(t, "blake3:aa", hashedEntry.Hash)
	assert.Empty(t, hashedEntry.Digest)
	assert.Empty(t, hashedEntry.DigestScheme)
}

func TestNormalize_DropsAnUnusableTimestamp(t *testing.T) {
	tests := []struct {
		name string
		ts   string
		want string
	}{
		{"valid", "2026-03-04T05:06:07Z", "2026-03-04T05:06:07Z"},
		{"empty", "", ""},
		{"not a time", "yesterday", ""},
		{"beyond the nanosecond range", "9999-01-01T00:00:00Z", ""},
		{"before the nanosecond range", "1000-01-01T00:00:00Z", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := Event{Kind: KindRule, ID: "r", Source: SourceHook, Outcome: OutcomeLoaded, Time: tt.ts}

			require.NoError(t, e.Normalize())

			assert.Equal(t, tt.want, e.Time)
		})
	}
}
