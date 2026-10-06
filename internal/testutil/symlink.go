// Package testutil holds helpers shared by tests across packages.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// RequireSymlink skips the test when the platform refuses to create symlinks
// (for example Windows without Developer Mode or the symlink privilege). It
// probes the capability instead of checking GOOS so privileged runners still
// run the test. When AI_RULEZ_REQUIRE_SYMLINKS is set (the privileged CI
// jobs), a refusal fails the test instead, so the symlink safety checks cannot
// silently stop running there.
func RequireSymlink(tb testing.TB) {
	tb.Helper()
	dir := tb.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		tb.Fatalf("symlink probe: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "link")); err != nil {
		if os.Getenv("AI_RULEZ_REQUIRE_SYMLINKS") != "" {
			tb.Fatalf("symlinks required but unavailable: %v", err)
		}
		tb.Skipf("symlinks not supported on this platform: %v", err)
	}
}

// SymlinkOrSkip creates a symlink at newname pointing to oldname. It skips the
// test when the platform cannot create symlinks and fails it on any other error.
func SymlinkOrSkip(tb testing.TB, oldname, newname string) {
	tb.Helper()
	RequireSymlink(tb)
	if err := os.Symlink(oldname, newname); err != nil {
		tb.Fatalf("symlink %s -> %s: %v", newname, oldname, err)
	}
}
