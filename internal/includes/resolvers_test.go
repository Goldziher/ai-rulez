package includes

import (
	"context"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
)

// loadWithResolvers loads a configuration the way the commands do, with the
// include and installed-skill resolvers wired in.
// isolateHome gives the test a home directory of its own, so the include cache
// (under the home directory) starts empty. Windows reads USERPROFILE, not HOME.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// lockPolicy is the lock policy loadWithResolvers loads under; a test sets the
// fields it needs and resets it.
var lockPolicy config.LockPolicy

// testOKFScan is the OKF include scan loadWithResolvers wires; a test sets the
// fields it needs and resets it.
var testOKFScan okfbridge.Scanner

func loadWithResolvers(ctx context.Context, baseDir string, opts ...config.LoadOption) (*config.Config, error) {
	return config.LoadConfig(ctx, baseDir, append([]config.LoadOption{config.WithResolvers(Resolvers("", testOKFScan)), config.WithLockPolicy(lockPolicy)}, opts...)...)
}
