package golden

import (
	"runtime"
	"sort"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	_ "github.com/Goldziher/ai-rulez/v5/internal/generator" // registers the presets
)

// TestPresetCount pins the number of presets the matrix covers, so a new preset
// cannot slip in without a golden.
func TestPresetCount(t *testing.T) {
	if got := len(config.IndividualPresetNames()); got != 52 {
		t.Fatalf("the matrix was recorded for 52 presets, the registry has %d: add a golden for the new one and update this count", got)
	}
}

// TestGoldens runs every scenario and compares it to its golden.
func TestGoldens(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("goldens record POSIX modes and paths")
	}
	scenarios := allScenarios()
	sort.Slice(scenarios, func(i, j int) bool { return scenarios[i].name < scenarios[j].name })
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			t.Parallel()
			compare(t, sc.name, execute(t, sc))
		})
	}
}
