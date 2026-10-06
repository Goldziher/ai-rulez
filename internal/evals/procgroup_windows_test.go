//go:build windows

package evals

import "testing"

// processGone is only reached by POSIX-script tests, which skip on Windows.
func processGone(t *testing.T, _ string) bool {
	t.Helper()
	return true
}
