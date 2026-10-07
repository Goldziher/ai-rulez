package catalogsite

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The probe decides whether the browser tests run, so a skip must say exactly why
// the browser did not start; these fakes stand in for the ways a real one fails.
func TestChromeProbe_SaysWhyTheBrowserDoesNotStart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake browsers are shell scripts")
	}
	tests := []struct {
		name    string
		script  string
		wantErr []string
	}{
		{"starts", `case "$1" in --version) echo "Chromium 1.0";; *) echo "<html></html>";; esac`, nil},
		{"killed at exec", `kill -9 $$`, []string{"--version", "signal: killed"}},
		{"no version", `echo "missing libnss3.so" >&2; exit 127`, []string{"--version", "exit status 127", "missing libnss3.so"}},
		{"no sandbox", `case "$1" in --version) echo "Chromium 1.0";; *) echo "No usable sandbox!" >&2; exit 1;; esac`,
			[]string{"headless", "exit status 1", "No usable sandbox!"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			bin := filepath.Join(t.TempDir(), "chrome")
			require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"+tt.script+"\n"), 0o755)) //nolint:gosec // a fake browser

			// Act
			err := chromeProbe(bin)

			// Assert
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, want := range tt.wantErr {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}
