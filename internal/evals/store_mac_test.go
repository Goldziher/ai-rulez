package evals

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var macKey = []byte("0123456789abcdef0123456789abcdef")

func TestStoreMAC_OnlyRecordsSignedByThisUsersKeyAreVerified(t *testing.T) {
	tests := []struct {
		name     string
		key      []byte
		tamper   func(data string) string
		verified bool
	}{
		{"signed by this key", macKey, func(s string) string { return s }, true},
		{"different key", []byte("ffffffffffffffffffffffffffffffff"), func(s string) string { return s }, false},
		{"no key available", nil, func(s string) string { return s }, false},
		{"edited after signing", macKey, func(s string) string { return replaceOnce(s, `"passing": false`, `"passing": true`) }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := filepath.Join(t.TempDir(), "eval-results.json")
			store := NewStore()
			store.SetKey(macKey)
			store.Put(SkillRecord{ID: "a", Digest: "sha256:1", CacheKey: "k"})
			require.NoError(t, store.Save(path))
			data, err := os.ReadFile(path) //nolint:gosec // test
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, []byte(tt.tamper(string(data))), 0o600))

			// Act
			loaded, err := LoadStoreKeyed(path, tt.key)

			// Assert
			require.NoError(t, err)
			rec, ok := loaded.Get("a")
			require.True(t, ok)
			assert.Equal(t, tt.verified, rec.Verified())
		})
	}
}

func replaceOnce(s, old, repl string) string {
	for i := 0; i+len(old) <= len(s); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + repl + s[i+len(old):]
		}
	}
	return s
}

func TestRun_UnverifiedStoredResultIsRerunNotReplayed(t *testing.T) {
	// Arrange: a result recorded (and committed) by someone else, signed with another key.
	cfg := projectWithSkills(t)
	path := filepath.Join(cfg, StoreFileName)
	runner := goodRunner()
	other := baseOptions(cfg, runner)
	other.Store.SetKey([]byte("ffffffffffffffffffffffffffffffff"))
	_, err := Run(context.Background(), other)
	require.NoError(t, err)
	require.NoError(t, other.Store.Save(path))
	require.Equal(t, 2, runner.calls)

	// Act: this user loads the committed file with their own key and runs again.
	mine, err := LoadStoreKeyed(path, macKey)
	require.NoError(t, err)
	opts := baseOptions(cfg, runner)
	opts.Store = mine
	report, err := Run(context.Background(), opts)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 4, runner.calls, "unverified records never satisfy a cache hit")
	assert.Equal(t, RunRan, report.Skills[0].Status)
	assert.Contains(t, report.Skills[0].Warnings[0], "unverified")

	// ...and the fresh result is signed, so the next run is cached.
	_, err = Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 4, runner.calls)
}

func TestRun_ForgedPassingRecordWithMatchingDigestsStillRuns(t *testing.T) {
	cfg := projectWithSkills(t)
	runner := goodRunner()
	probe := baseOptions(cfg, runner)
	_, err := Run(context.Background(), probe)
	require.NoError(t, err)
	forged := NewStore()
	for _, rec := range probe.Store.Skills {
		rec.Passing = true
		rec.MAC = "hmac-sha256:deadbeef"
		forged.Skills = append(forged.Skills, rec)
	}
	path := filepath.Join(cfg, StoreFileName)
	require.NoError(t, forged.Save(path))
	calls := runner.calls

	store, err := LoadStoreKeyed(path, macKey)
	require.NoError(t, err)
	opts := baseOptions(cfg, runner)
	opts.Store = store
	_, err = Run(context.Background(), opts)

	require.NoError(t, err)
	assert.Equal(t, calls+2, runner.calls)
}
