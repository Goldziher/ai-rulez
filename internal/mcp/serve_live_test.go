package mcp

import (
	"context"
	"errors"
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
