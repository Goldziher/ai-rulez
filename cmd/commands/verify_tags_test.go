package commands

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func lockedAgeFixture(t *testing.T, lockTable string) *ageFixture {
	t.Helper()
	f := newAgeFixture(t, lockTable, `version = "^1"`)
	t.Cleanup(func() { lockVerifyTags, generateVerifyTags = false, false })
	require.Equal(t, 0, writeLockAt("", "", nil))
	require.Equal(t, "v1.2.0", f.lock().Find(lockfile.KindInclude, "shared").Tag)
	lockCheck = true
	return f
}

func TestLockCheckVerifyTags(t *testing.T) {
	tests := []struct {
		name       string
		table      string
		verify     bool
		mutate     func(f *ageFixture)
		wantCode   int
		wantStderr string
	}{
		{name: "tags unchanged", verify: true, mutate: func(*ageFixture) {}, wantCode: 0},
		{
			name: "a moved tag is AR732", verify: true, wantCode: exitDrift, wantStderr: "AR732",
			mutate: func(f *ageFixture) { f.release("evil", "v1.2.0", true) },
		},
		{
			name: "a deleted tag only warns (AR735) and the check passes", verify: true, wantCode: 0,
			mutate: func(f *ageFixture) { f.repo.DeleteTag("v1.2.0") },
		},
		{
			name: "the [lock] verify_tags key alone asks the remote, as the docs say", table: "[lock]\nverify_tags = true\n",
			wantCode: exitDrift, wantStderr: "AR732",
			mutate: func(f *ageFixture) { f.release("evil", "v1.2.0", true) },
		},
		{
			name: "without --verify-tags the offline check does not look (the default is unchanged)", verify: false, wantCode: 0,
			mutate: func(f *ageFixture) { f.release("evil", "v1.2.0", true) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := lockedAgeFixture(t, tt.table)
			tt.mutate(f)
			lockVerifyTags = tt.verify

			// Act
			var code int
			_, stderr := capture(t, func() { code = checkLockAt("") })

			// Assert
			assert.Equal(t, tt.wantCode, code)
			if tt.wantStderr != "" {
				assert.Contains(t, stderr, tt.wantStderr)
				assert.Contains(t, stderr, "ai-rulez update --accept-moved-tag")
			}
		})
	}
}

func TestLockCheckVerifyTagsFindsTheMovedTagItself(t *testing.T) {
	f := lockedAgeFixture(t, "")
	was := f.lock().Find(lockfile.KindInclude, "shared").Commit
	now := f.release("evil", "v1.2.0", true)
	require.NotEqual(t, was, now)
	cfg, _, err := loadForLockCheck("")
	require.NoError(t, err)

	findings, err := verifyPinnedTags(context.Background(), cfg, f.lock())

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "AR732", findings[0].code)
	assert.True(t, findings[0].failing)
	assert.Contains(t, findings[0].message, shortSHA(was))
	assert.Contains(t, findings[0].message, shortSHA(now))
}

func TestVerifyTagsWanted(t *testing.T) {
	prev := includes.SkipFetch
	t.Cleanup(func() { includes.SkipFetch, lockOffline = prev, false })
	on := &config.Config{Lock: &config.LockConfig{VerifyTags: true}}
	off := &config.Config{}
	tests := []struct {
		name     string
		cfg      *config.Config
		flag     bool
		offline  bool
		want     bool
		wantFail bool
	}{
		{"nothing asks", off, false, false, false, false},
		{"flag", off, true, false, true, false},
		{"config key", on, false, false, true, false},
		{"config key but offline run is skipped quietly", on, false, true, false, false},
		{"flag but offline run is a usage error", off, true, true, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			includes.SkipFetch = tt.offline

			got, err := verifyTagsWanted(tt.cfg, tt.flag)

			if tt.wantFail {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "needs the network")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateLockFlagsVerifyTagsNeedsCheck(t *testing.T) {
	t.Cleanup(func() { lockVerifyTags, lockCheck = false, false })
	lockVerifyTags, lockCheck = true, false
	require.ErrorContains(t, validateLockFlags(nil), "--verify-tags only applies to --check")
	lockCheck = true
	assert.NoError(t, validateLockFlags(nil))
}
