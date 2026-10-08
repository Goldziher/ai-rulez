//go:build windows

package evals

import (
	"context"
	"testing"
)

// processGone is only reached by POSIX-script tests, which skip on Windows.
func processGone(t *testing.T, _ string) bool {
	t.Helper()
	return true
}

func cancelWhenReady(_ string, cancel context.CancelFunc) { cancel() }
