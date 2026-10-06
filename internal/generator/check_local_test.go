package generator

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// A check run treats machine-local inputs as generate does: refuses local drift
// of a tracked output, and does not call the files of a skipped overlay orphans.
func TestCheckDriftRefusesLocalDriftLikeGenerate(t *testing.T) {
	// Arrange
	p := newDriftProject(t, strings.Replace(driftShared, "gitignore = false", "gitignore = true", 1))
	p.git(t, "init", "-q")
	require.NoError(t, NewGenerator(p.load(t, config.WithoutLocal())).Generate(""))
	p.git(t, "add", "-f", "CLAUDE.md")
	p.overlay(t, "name = \"mine\"\n")

	// Act
	generateErr := NewGenerator(p.load(t)).Generate("")
	_, checkErr := NewGenerator(p.load(t)).CheckDrift("")

	// Assert
	require.Error(t, generateErr, "precondition: generate refuses this local drift")
	require.Error(t, checkErr, "check refuses what generate would refuse")
}

func TestCheckDriftKeepsFilesOfASkippedLocalOverlay(t *testing.T) {
	// Arrange: an overlay adds the devin preset; the files it wrote are kept by a --no-local run.
	p := newDriftProject(t, strings.Replace(driftShared, `presets = ["claude"]`, `presets = ["claude", "cursor"]`, 1))
	p.git(t, "init", "-q")
	p.overlay(t, "presets = [\"devin\"]\n")
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))
	require.NoError(t, NewGenerator(p.load(t, config.WithoutLocal())).Generate(""))

	// Act
	drift, err := NewGenerator(p.load(t, config.WithoutLocal())).CheckDrift("")

	// Assert
	require.NoError(t, err)
	for _, d := range drift {
		assert.NotEqual(t, DriftOrphan, d.Kind, "a file kept for a skipped local input is not an orphan: %+v", d)
	}
}
