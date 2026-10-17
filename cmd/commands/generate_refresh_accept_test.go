package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestRefreshRemoteLock_AcceptFindings(t *testing.T) {
	tests := []struct {
		name    string
		accept  bool
		wantTag string
		wantErr bool
	}{
		{name: "a refused scan names the flag generate has", wantTag: "v1.1.0", wantErr: true},
		{name: "--accept-findings pins the tree anyway", accept: true, wantTag: "v1.2.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			flag := GenerateCmd.Flags().Lookup("accept-findings")
			require.NotNil(t, flag, "generate has the flag its refusal names")
			f := newUpdateFixture(t, `version = "^1"`)
			f.repo.Write(".ai-rulez/rules/leak.md", leakyRule)
			f.release("one point two", "v1.2.0", false)
			applyLockFlags()

			// Act
			var err error
			_, _ = capture(t, func() { _, err = refreshRemoteLockWith(cmdContext(), tt.accept) })

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "--accept-findings")
			} else {
				require.NoError(t, err)
			}
			lock, loadErr := lockfile.Load(filepath.Join(f.root, ".ai-rulez"))
			require.NoError(t, loadErr)
			assert.Equal(t, tt.wantTag, lock.Find(lockfile.KindInclude, "shared").Tag)
		})
	}
}
