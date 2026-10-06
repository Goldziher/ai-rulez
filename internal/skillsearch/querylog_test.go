package skillsearch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryLog_RecordsAndReads(t *testing.T) {
	t.Parallel()
	// Arrange
	path := filepath.Join(t.TempDir(), "local", QueryLogFile)
	l := &QueryLog{Path: path, Now: func() time.Time { return time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC) }}

	// Act
	l.Query("session-1", "  customer wants   money back ", ModeHybrid, []string{"a", "b", "c", "d", "e", "f"})
	l.Loaded("session-1", "issue-refund")
	got, err := ReadLog(path)

	// Assert
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "customer wants money back", got[0].Query)
	assert.Equal(t, []string{"a", "b", "c", "d", "e"}, got[0].Results, "the log keeps the top five")
	assert.Equal(t, "2026-10-06T12:00:00Z", got[0].Time)
	assert.Equal(t, got[0].Session, got[1].Session)
	assert.NotContains(t, got[0].Session, "session-1", "the session id is hashed")
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestQueryLog_NilAndSecrets(t *testing.T) {
	t.Parallel()
	var nilLog *QueryLog
	nilLog.Query("s", "q", "", nil)
	nilLog.Loaded("s", "x")

	path := filepath.Join(t.TempDir(), QueryLogFile)
	l := &QueryLog{Path: path, Scanner: func(s string) (string, bool) { return "key", strings.Contains(s, "AKIA") }}
	l.Query("s", "use key AKIAABCDEFGHIJKLMNOP now", "", nil)
	l.Query("s", "   ", "", nil)

	got, err := ReadLog(path)
	require.NoError(t, err)
	assert.Empty(t, got, "a query with a secret is never recorded")
}

func TestQueryLog_StopsAtTheSizeCap(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), QueryLogFile)
	require.NoError(t, os.WriteFile(path, make([]byte, maxLogBytes+1), 0o600))
	l := &QueryLog{Path: path}

	l.Query("s", "more", "", nil)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.EqualValues(t, maxLogBytes+1, info.Size())
}

func TestReadLog_SkipsGarbageAndMissing(t *testing.T) {
	t.Parallel()
	got, err := ReadLog(filepath.Join(t.TempDir(), "none"))
	require.NoError(t, err)
	assert.Nil(t, got)
	path := filepath.Join(t.TempDir(), QueryLogFile)
	require.NoError(t, os.WriteFile(path, []byte("not json\n{\"event\":\"other\"}\n{\"event\":\"query\",\"query\":\"q\"}\n"), 0o600))
	got, err = ReadLog(path)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestMine(t *testing.T) {
	t.Parallel()
	q := func(s, query string) LogEntry { return LogEntry{Event: eventQuery, Session: s, Query: query} }
	ld := func(s, skill string) LogEntry { return LogEntry{Event: eventLoaded, Session: s, Skill: skill} }
	entries := []LogEntry{
		q("s1", "money back"), ld("s1", "issue-refund"), ld("s1", "dispute-charge"), // the first load labels
		q("s2", "Money   back"), ld("s2", "issue-refund"), // same query, same label
		q("s1", "rollout to prod"), // no load: unlabelled
		q("s3", "which skill"), ld("s3", "a-skill"),
		q("s4", "which skill"), ld("s4", "b-skill"), // ambiguous
		q("s5", "unknown label"), ld("s5", "ghost"), // loaded skill is not known
		q("s6", "once"), ld("s6", "issue-refund"), // seen once
	}
	known := []string{"issue-refund", "dispute-charge", "a-skill", "b-skill"}

	t.Run("min count 1", func(t *testing.T) {
		m := Mine(entries, MineOptions{Known: known})
		assert.Equal(t, 5, m.Queries)
		assert.Equal(t, 2, m.Unlabelled, "rollout, and the unknown label")
		assert.Equal(t, 1, m.Ambiguous)
		require.Len(t, m.Cases, 2)
		assert.Equal(t, "money back", m.Cases[0].Query)
		assert.Equal(t, []string{"issue-refund"}, m.Cases[0].ExpectIDs())
		assert.Equal(t, []string{"mined"}, m.Cases[0].Tags)
		assert.Regexp(t, `^mined-[0-9a-f]{8}$`, m.Cases[0].ID)
	})
	t.Run("min count 2", func(t *testing.T) {
		m := Mine(entries, MineOptions{Known: known, MinCount: 2})
		require.Len(t, m.Cases, 1)
		assert.Equal(t, "money back", m.Cases[0].Query)
		assert.Equal(t, 1, m.Below)
	})
}

func TestCasesYAML_RoundTrips(t *testing.T) {
	t.Parallel()
	cases := []Case{
		{ID: "a", Query: "money back", Expect: []Relevant{{ID: "issue-refund", Grade: 2, Graded: true}, {ID: "x", Grade: 1}}, Tags: []string{"mined"}},
		{ID: "n", Query: "weather", Avoid: []string{"x"}},
	}

	raw, err := CasesYAML(5, cases)
	require.NoError(t, err)
	got, err := ParseCases(raw)

	require.NoError(t, err)
	assert.Equal(t, 5, got.K)
	assert.Equal(t, cases[0].Expect, got.Cases[0].Expect)
	assert.Equal(t, []string{"x"}, got.Cases[1].Avoid)
	assert.Empty(t, got.Cases[1].Expect)
}

func TestQueryLog_NeverWritesThroughASymlink(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		link func(t *testing.T, dir, victim string) string // returns the log path
	}{
		{"symlinked log file", func(t *testing.T, dir, victim string) string {
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "local"), 0o750))
			path := filepath.Join(dir, "local", QueryLogFile)
			testutil.SymlinkOrSkip(t, victim, path)
			return path
		}},
		{"symlinked local directory", func(t *testing.T, dir, victim string) string {
			testutil.SymlinkOrSkip(t, filepath.Dir(victim), filepath.Join(dir, "local"))
			return filepath.Join(dir, "local", QueryLogFile)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := filepath.Join(t.TempDir(), ".ai-rulez")
			require.NoError(t, os.MkdirAll(dir, 0o750))
			victim := filepath.Join(t.TempDir(), "victim.txt")
			require.NoError(t, os.WriteFile(victim, []byte("keep\n"), 0o600))
			l := &QueryLog{Path: tt.link(t, dir, victim)}

			// Act
			l.Query("s", "customer wants money back", "", []string{"a"})

			// Assert
			got, err := os.ReadFile(victim)
			require.NoError(t, err)
			assert.Equal(t, "keep\n", string(got), "the symlink target must not be appended to")
			entries, err := os.ReadDir(filepath.Dir(victim))
			require.NoError(t, err)
			assert.Len(t, entries, 1, "nothing is created beside the victim")
		})
	}
}
