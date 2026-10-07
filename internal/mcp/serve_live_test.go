package mcp

import (
	"os"
	"path/filepath"

	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"

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

func TestServeSetup_InitialFingerprintAgreesWithTheWatcherForALogInTheConfigDir(t *testing.T) {
	// Arrange: a usage log and its salt inside the config directory, outside `local/`.
	root := project(t, baseConfig, map[string]string{
		"skills/core/SKILL.md": skillFile("core", "Core conventions", ""),
		"logs/usage.jsonl":     "line\n",
		"logs/usage.salt":      "salt\n",
	})
	logPath := filepath.Join(root, ".ai-rulez", "logs", "usage.jsonl")
	st := &ServeSetup{WorkDir: root, UsageLog: logPath}
	cfgDir, err := filepath.Abs(filepath.Join(root, ".ai-rulez"))
	require.NoError(t, err)

	// Act
	baseline, err := st.initialFingerprint()
	require.NoError(t, err)
	poll, err := fingerprint([]string{cfgDir}, st.usageFiles(&config.Config{})...)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, poll, baseline, "the first poll must not see a change that is only the usage files")
}

func TestServeSetup_UsageSinkRecordsCarryTheSaltedSessionAndDoNotBlock(t *testing.T) {
	tests := []struct {
		name        string
		sink        func(out string) string
		wantSession bool
		wantFast    bool
	}{
		{"sink only records the session like a log line", func(out string) string { return "cat >> '" + out + "'" }, true, true},
		{"a hanging sink does not delay the load", func(string) string { return "sleep 30" }, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			out := filepath.Join(dir, "sink.out")
			st := &ServeSetup{UsageSink: tt.sink(out)}
			record, closeSink := st.telemetry(&config.Config{ConfigDir: filepath.Join(dir, ".ai-rulez")})

			// Act
			start := time.Now()
			record(SessionTelemetry{Skill: "kit", Session: "conn-1", Client: "c"})
			elapsed := time.Since(start)
			if tt.wantSession {
				closeSink(5 * time.Second)
			} else {
				closeSink(0)
			}

			// Assert
			assert.Less(t, elapsed, time.Second, "load latency must not depend on the sink")
			if tt.wantSession {
				got, err := os.ReadFile(out)
				require.NoError(t, err)
				assert.Contains(t, string(got), `"session":"`)
				assert.Contains(t, string(got), `"v":3`)
			}
		})
	}
}
