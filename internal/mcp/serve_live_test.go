package mcp

import (
	"path/filepath"

	"context"
	"errors"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A rebuild that fails (the network was down) is retried on the next tick even
// though the files have not changed again.
func TestWatch_RetriesAFailedRebuildWithoutAnotherEdit(t *testing.T) {
	t.Parallel()
	var fingerprint atomic.Value
	fingerprint.Store("v1")
	var attempts atomic.Int32
	reloaded := make(chan struct{}, 1)
	next := loadCatalog(t)

	srv := NewSkillServerWith("test", loadCatalog(t), ServeOptions{
		PollInterval: 10 * time.Millisecond,
		Baseline:     "v1",
		Fingerprint:  func() (string, error) { return fingerprint.Load().(string), nil },
		Rebuild: func() (*Catalog, error) {
			if attempts.Add(1) < 3 {
				return nil, errors.New("network is down")
			}
			select {
			case reloaded <- struct{}{}:
			default:
			}
			return next, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Watch(ctx)

	fingerprint.Store("v2") // one edit, then nothing changes
	select {
	case <-reloaded:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "the failed rebuild was never retried", "attempts: %d", attempts.Load())
	}
	assert.GreaterOrEqual(t, attempts.Load(), int32(3))
}

func TestFingerprint_UsageLogIsExcludedOnlyWhereTheServerWritesIt(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "skills/a/SKILL.md", "x")
	writeFile(t, dir, "skills/a/references/data.jsonl", "one\n")
	before, err := fingerprint([]string{dir})
	require.NoError(t, err)
	writeFile(t, dir, "skills/a/references/data.jsonl", "one\ntwo\n")
	after, err := fingerprint([]string{dir})
	require.NoError(t, err)
	assert.NotEqual(t, before, after, "a .jsonl resource of a skill is content")
}

// A new edit after a failed rebuild ends the retry pause: the user fixed the
// problem and should not wait out a backoff meant for an unchanged tree.
func TestWatch_NewEditEndsTheRetryPause(t *testing.T) {
	t.Parallel()
	var fingerprint atomic.Value
	fingerprint.Store("v1")
	var attempts atomic.Int32
	failed := make(chan struct{}, 1)
	second := make(chan struct{}, 1)
	srv := NewSkillServerWith("test", loadCatalog(t), ServeOptions{
		PollInterval: 300 * time.Millisecond,
		Baseline:     "v1",
		Fingerprint:  func() (string, error) { return fingerprint.Load().(string), nil },
		Rebuild: func() (*Catalog, error) {
			if attempts.Add(1) == 1 {
				failed <- struct{}{}
				return nil, errors.New("broken edit")
			}
			second <- struct{}{}
			return loadCatalog(t), nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Watch(ctx)

	fingerprint.Store("v2")
	<-failed
	fingerprint.Store("v3") // the fix

	select {
	case <-second:
	case <-time.After(450 * time.Millisecond):
		require.FailNow(t, "the new edit waited out the retry pause")
	}
}

func TestFingerprint_UsageLogAndSaltAreExcludedByAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "skills/a/SKILL.md", "x")
	logs := filepath.Join(dir, "logs", "usage.jsonl")
	salt := filepath.Join(dir, "logs", "usage.salt")
	before, err := fingerprint([]string{dir}, logs, salt)
	require.NoError(t, err)

	writeFile(t, dir, "logs/usage.jsonl", "line\n")
	writeFile(t, dir, "logs/usage.salt", "salt\n")
	writeFile(t, dir, "local/usage.salt", "salt\n")
	after, err := fingerprint([]string{dir}, logs, salt)

	require.NoError(t, err)
	assert.Equal(t, before, after, "writing the usage log and its salt must not trigger a reload")
}

func TestServeSetup_UsageFilesAreAbsoluteAndIncludeTheSalt(t *testing.T) {
	st := &ServeSetup{UsageLog: filepath.Join("rel", "u.jsonl")}
	got := st.usageFiles(&config.Config{})
	require.Len(t, got, 2)
	for _, p := range got {
		assert.True(t, filepath.IsAbs(p), p)
	}
	assert.Equal(t, "usage.salt", filepath.Base(got[1]))
}
