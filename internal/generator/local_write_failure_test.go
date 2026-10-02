package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Machine-local outputs are only written when the bookkeeping that keeps them out
// of git and lets a later run clean them up can be written too.
func TestGenerate_LocalBookkeepingFailuresAreErrors(t *testing.T) {
	tests := []struct {
		name    string
		breakIt func(t *testing.T, p *driftProject)
	}{
		{"local manifest cannot be written", func(t *testing.T, p *driftProject) {
			require.NoError(t, os.MkdirAll(filepath.Join(p.dir, ".generated-manifest.local.json", "blocker"), 0o755))
		}},
		{".gitignore cannot be updated", func(t *testing.T, p *driftProject) {
			require.NoError(t, os.MkdirAll(filepath.Join(p.base, ".gitignore", "blocker"), 0o755))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := newDriftProject(t, strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1))
			p.overlay(t, "presets = [\"codex\"]\n")
			tt.breakIt(t, p)

			// Act
			err := NewGenerator(p.load(t)).Generate("")

			// Assert
			require.Error(t, err)
		})
	}
}

func TestDryRun_BlockedLinesNameThePathAndReasonOnly(t *testing.T) {
	// Arrange
	p := newDriftProject(t, driftShared)
	p.overlay(t, "name = \"PRIVATE-NAME\"\n")

	// Act
	plan, err := NewGenerator(p.load(t)).DryRun("")

	// Assert
	require.NoError(t, err)
	var blocked []string
	for _, line := range plan {
		if strings.HasPrefix(line, "blocked: ") {
			blocked = append(blocked, line)
		}
		assert.NotContains(t, line, "PRIVATE-NAME")
	}
	assert.Equal(t, []string{"blocked: CLAUDE.md (shared output would change and is not git-ignored)"}, blocked)
}
