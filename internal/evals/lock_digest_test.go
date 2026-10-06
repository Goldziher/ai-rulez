package evals

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_RecordsTheLockDigestOfTheSkill(t *testing.T) {
	cfg := projectWithSkills(t)
	opts := baseOptions(cfg, goodRunner())

	report, err := Run(context.Background(), opts)

	require.NoError(t, err)
	require.NotEmpty(t, report.Skills)
	want, err := contentlock.SkillDirDigest(filepath.Join(cfg, "skills", "alpha"))
	require.NoError(t, err)
	record, ok := opts.Store.Get("alpha")
	require.True(t, ok)
	assert.Equal(t, want, record.LockDigest, "the eval record carries the lock's digest of the skill")
	assert.NotEqual(t, record.Digest, record.LockDigest, "the cache digest keeps its own scheme")
}

func TestRun_EditingACaseKeepsTheLockDigest(t *testing.T) {
	cfg := projectWithSkills(t)
	dir := filepath.Join(cfg, "skills", "alpha")
	before, err := contentlock.SkillDirDigest(dir)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "evals", "main.eval.yaml"), []byte(twoCases+"# edited\n"), 0o600))
	after, err := contentlock.SkillDirDigest(dir)

	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestRun_CachedRecordWithoutLockDigestIsBackfilled(t *testing.T) {
	cfg := projectWithSkills(t)
	opts := baseOptions(cfg, goodRunner())
	_, err := Run(context.Background(), opts)
	require.NoError(t, err)
	record, _ := opts.Store.Get("alpha")
	want := record.LockDigest
	require.NotEmpty(t, want)
	record.LockDigest = "" // a record written before lock_digest existed

	report, err := Run(context.Background(), opts)

	require.NoError(t, err)
	assert.Equal(t, RunCached, report.Skills[0].Status)
	record, _ = opts.Store.Get("alpha")
	assert.Equal(t, want, record.LockDigest)
}

func TestStoreMarshal_OmitsAnEmptyLockDigest(t *testing.T) {
	store := NewStore()
	store.Put(SkillRecord{ID: "a", Digest: "sha256:x"})

	data, err := store.Marshal()

	require.NoError(t, err)
	assert.NotContains(t, string(data), "lock_digest", "a legacy record keeps its bytes")
}

func TestRun_WarnsWhenTheLockDigestCannotBeComputed(t *testing.T) {
	cfg := projectWithSkills(t)
	previous := lockDigestOf
	lockDigestOf = func(string) (string, error) { return "", errors.New("walk failed") }
	t.Cleanup(func() { lockDigestOf = previous })

	report, err := Run(context.Background(), baseOptions(cfg, goodRunner()))

	require.NoError(t, err)
	var warned bool
	for _, sk := range report.Skills {
		for _, w := range sk.Warnings {
			warned = warned || (strings.Contains(w, "lock digest") && strings.Contains(w, "walk failed"))
		}
	}
	assert.True(t, warned, "the usage join silently degraded to by-id only; say so")
}
