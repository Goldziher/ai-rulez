package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

const leakyRule = "# Leak\naws_secret_access_key = \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYQ9d3kGh7Lz\"\nAKIAQYLPMN5HHHFPZAM2\n"

func TestLock_PrePinScan(t *testing.T) {
	tests := []struct {
		name       string
		leak       bool
		accept     bool
		wantCode   int
		wantPinned bool
	}{
		{"a clean tree is pinned", false, false, 0, true},
		{"error findings refuse the pin", true, false, exitDrift, false},
		{"--accept-findings pins it anyway", true, true, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: the newest tag carries a secret-shaped string when leak is set.
			f := newAgeFixture(t, "", `version = "~1.1.0"`)
			if tt.leak {
				f.repo.Write(".ai-rulez/rules/leak.md", leakyRule)
				f.release("leaky", "v1.1.1", false)
			}
			lockAcceptFindings = tt.accept
			t.Cleanup(func() { lockAcceptFindings = false })

			// Act
			var code int
			_, stderr := capture(t, func() { code = writeLockAt("", "", nil) })

			// Assert
			assert.Equal(t, tt.wantCode, code, stderr)
			lock, err := lockfile.Load(filepath.Join(f.root, ".ai-rulez"))
			require.NoError(t, err)
			if !tt.wantPinned {
				assert.Contains(t, stderr, "--accept-findings")
				assert.Nil(t, lock, "a refused scan writes no lock")
				return
			}
			require.NotNil(t, lock.Find(lockfile.KindInclude, "shared"))
		})
	}
}

func TestLock_PrePinScanSkipsAnUnchangedPin(t *testing.T) {
	// Arrange: pin a clean tree, then let the remote grow a leaky tag outside the range.
	f := newAgeFixture(t, "", `version = "~1.1.0"`)
	require.Equal(t, 0, writeLockAt("", "", nil))
	before := f.lock()
	f.repo.Write(".ai-rulez/rules/leak.md", leakyRule)
	f.release("leaky", "v1.1.1", false)

	// Act: the pin still satisfies the range, so it is kept and nothing new is pinned.
	code := writeLockAt("", "", nil)

	// Assert
	assert.Equal(t, 0, code)
	assert.Equal(t, before.Find(lockfile.KindInclude, "shared").Commit, f.lock().Find(lockfile.KindInclude, "shared").Commit)
}
