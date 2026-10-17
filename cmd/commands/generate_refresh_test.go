package commands

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestRefreshRemoteLock(t *testing.T) {
	tests := []struct {
		name     string
		leak     bool
		wantTag  string
		wantCode int
	}{
		{name: "moves the pin to the newest allowed tag", wantTag: "v1.2.0"},
		{name: "a refused scan aborts and keeps the lock", leak: true, wantTag: "v1.1.0", wantCode: exitFindings},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: locked at v1.1.0 under ^1, then the remote grows v1.2.0.
			f := newUpdateFixture(t, `version = "^1"`)
			if tt.leak {
				f.repo.Write(".ai-rulez/rules/leak.md", leakyRule)
			}
			f.release("one point two", "v1.2.0", false)
			applyLockFlags()

			// Act
			var err error
			_, _ = capture(t, func() { err = refreshRemoteLock(cmdContext()) })

			// Assert
			if tt.wantCode != 0 {
				require.Error(t, err)
				var exit *ExitError
				require.True(t, errors.As(err, &exit))
				assert.Equal(t, tt.wantCode, exit.Code)
			} else {
				require.NoError(t, err)
			}
			lock, loadErr := lockfile.Load(filepath.Join(f.root, ".ai-rulez"))
			require.NoError(t, loadErr)
			assert.Equal(t, tt.wantTag, lock.Find(lockfile.KindInclude, "shared").Tag)
		})
	}
}

func TestRefreshBlockedBy(t *testing.T) {
	tests := []struct {
		name string
		set  func(t *testing.T)
		want string
	}{
		{name: "nothing set", set: func(*testing.T) {}},
		{name: "check", set: func(t *testing.T) { setGenerateFlag(t, &generateCheck) }, want: "--check"},
		{name: "dry run", set: func(t *testing.T) { setGenerateFlag(t, &dryRun) }, want: "--dry-run"},
		{name: "locked", set: func(t *testing.T) { setGenerateFlag(t, &generateLocked) }, want: "--locked"},
		{name: "frozen", set: func(t *testing.T) { setGenerateFlag(t, &generateFrozen) }, want: "--frozen"},
		{name: "offline", set: func(t *testing.T) { setGenerateFlag(t, &noFetch) }, want: "--offline"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			tt.set(t)

			// Act
			got := refreshBlockedBy()

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRefreshOnGenerateNeedsTheConfigKey(t *testing.T) {
	assert.False(t, refreshOnGenerate(nil))
	assert.False(t, refreshOnGenerate(&config.Config{}))
	assert.False(t, refreshOnGenerate(&config.Config{Lock: &config.LockConfig{}}))
	assert.True(t, refreshOnGenerate(&config.Config{Lock: &config.LockConfig{RefreshOnGenerate: true}}))

	setGenerateFlag(t, &generateLocked)
	assert.False(t, refreshOnGenerate(&config.Config{Lock: &config.LockConfig{RefreshOnGenerate: true}}), "--locked wins")
}

func TestCheckRefreshFlagsRejectsContradictions(t *testing.T) {
	require.NoError(t, GenerateCmd.Flags().Set(flagRefreshRemote, "true"))
	t.Cleanup(func() { require.NoError(t, GenerateCmd.Flags().Set(flagRefreshRemote, "false")) })

	require.NoError(t, checkRefreshFlags(GenerateCmd))
	setGenerateFlag(t, &generateFrozen)
	err := checkRefreshFlags(GenerateCmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--frozen")
}

// setGenerateFlag turns a generate flag variable on for one test.
func setGenerateFlag(t *testing.T, flag *bool) {
	t.Helper()
	*flag = true
	t.Cleanup(func() { *flag = false })
}
