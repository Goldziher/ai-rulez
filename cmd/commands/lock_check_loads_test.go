package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
)

// TestCheckLockAtLoadsTheConfigurationOnce guards the single load that serves the
// content comparison, the tag check and the signature check of `lock --check`.
func TestCheckLockAtLoadsTheConfigurationOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cliLockPolicy.Mode, cliLockPolicy.Offline = includes.LockAuto, false
	t.Cleanup(func() { cliLockPolicy.Mode, cliLockPolicy.Offline = includes.LockAuto, false })
	resetLockViewFlags(t)
	lockProject(t, "")
	require.Equal(t, 0, writeLockAt("", "", nil))

	real := loadWithCacheFallback
	loads := 0
	loadWithCacheFallback = func(load func(opts ...config.LoadOption) (*config.Config, error)) (*config.Config, bool, error) {
		loads++
		return real(load)
	}
	t.Cleanup(func() { loadWithCacheFallback = real })

	var code int
	capture(t, func() { code = checkLockAt("") })

	assert.Equal(t, 0, code)
	assert.Equal(t, 1, loads, "lock --check loaded the configuration more than once")
}
