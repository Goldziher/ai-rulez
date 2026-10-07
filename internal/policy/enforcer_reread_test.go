package policy

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

// TestEnforcerRereadsAnEditedPolicyFile: a long-running process (the skills
// server) keeps one enforcer; an edit to a local policy file, a managed file that
// appears, or an edited extends target must be seen on the next load.
func TestEnforcerRereadsAnEditedPolicyFile(t *testing.T) {
	locked := "policy_version = 1\n[telemetry]\nallow_network = false\n"
	open := "policy_version = 1\n[lock]\nenforce = true\n"
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string) (flag string, managed string, edit func())
		want  bool
	}{
		{
			name: "the flag's file is edited in place, same size and mtime",
			setup: func(t *testing.T, dir string) (string, string, func()) {
				p := writePolicy(t, dir, "p.toml", open)
				info, err := os.Stat(p)
				require.NoError(t, err)
				return p, filepath.Join(dir, "none.toml"), func() {
					body := "policy_version = 1\n[telemetry]\nallow_network = false\n"
					require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
					require.NoError(t, os.Chtimes(p, info.ModTime(), info.ModTime()))
				}
			},
			want: true,
		},
		{
			name: "a managed policy file appears",
			setup: func(t *testing.T, dir string) (string, string, func()) {
				managed := filepath.Join(dir, "managed.toml")
				return "", managed, func() { writePolicy(t, dir, "managed.toml", locked) }
			},
			want: true,
		},
		{
			name: "an extended file is edited",
			setup: func(t *testing.T, dir string) (string, string, func()) {
				base := writePolicy(t, dir, "base.toml", open)
				child := writePolicy(t, dir, "child.toml", "policy_version = 1\nextends = [\"base.toml\"]\n")
				return child, filepath.Join(dir, "none.toml"), func() {
					require.NoError(t, os.WriteFile(base, []byte(locked), 0o600))
					later := time.Now().Add(time.Second)
					require.NoError(t, os.Chtimes(base, later, later))
				}
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			flag, managed, edit := tt.setup(t, dir)
			e := NewEnforcer(func() DiscoverOptions {
				return DiscoverOptions{Flag: flag, Env: ambient.MapEnv{}, ManagedPaths: []string{managed}}
			})
			require.False(t, e.Locks("telemetry"))

			// Act
			edit()

			// Assert
			assert.Equal(t, tt.want, e.Locks("telemetry"))
		})
	}
}
