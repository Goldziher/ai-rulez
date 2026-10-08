//go:build !windows

package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// envSpawnProbe makes the test binary act as the re-executed flush child: it
// records how it was started and exits, so spawnFlush is tested without the
// real CLI and without any network.
const envSpawnProbe = "AI_RULEZ_TEST_SPAWN_PROBE"

type spawnProbe struct {
	Args      []string `json:"args"`
	SessionID int      `json:"session"`
	PID       int      `json:"pid"`
	StdinNull bool     `json:"stdin_null"`
}

func TestMain(m *testing.M) {
	if out := os.Getenv(envSpawnProbe); out != "" {
		sid, _ := syscall.Getsid(0) //nolint:errcheck // reported as 0
		in, inErr := os.Stdin.Stat()
		null, nullErr := os.Stat(os.DevNull)
		probe := spawnProbe{Args: os.Args[1:], SessionID: sid, PID: os.Getpid(), StdinNull: inErr == nil && nullErr == nil && os.SameFile(in, null)}
		data, _ := json.Marshal(probe) //nolint:errcheck // plain struct
		_ = os.WriteFile(out+".tmp", data, 0o600)
		_ = os.Rename(out+".tmp", out)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestSpawnFlush_ReexecsADetachedFlush(t *testing.T) {
	tests := []struct {
		name       string
		configDir  string
		wantSuffix []string
	}{
		{name: "default config dir", wantSuffix: nil},
		{name: "named config dir", configDir: ".custom-rulez", wantSuffix: []string{"--config-dir", ".custom-rulez"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			out := filepath.Join(t.TempDir(), "probe.json")
			t.Setenv(envSpawnProbe, out)
			p := &Pipeline{Root: "/project/root", ConfigDirName: tt.configDir}

			// Act
			require.NoError(t, p.spawnFlush())

			// Assert
			var got spawnProbe
			require.Eventually(t, func() bool {
				data, err := os.ReadFile(out)
				return err == nil && json.Unmarshal(data, &got) == nil
			}, 30*time.Second, 10*time.Millisecond, "the detached child ran")
			want := append([]string{"telemetry", "flush", "--background", "--root", "/project/root"}, tt.wantSuffix...)
			assert.Equal(t, want, got.Args)
			assert.Equal(t, got.PID, got.SessionID, "the child leads its own session, so it outlives the hook")
			assert.NotEqual(t, syscall.Getpid(), got.PID)
			assert.True(t, got.StdinNull, "the child reads /dev/null, inheriting no terminal or pipe")
		})
	}
}
