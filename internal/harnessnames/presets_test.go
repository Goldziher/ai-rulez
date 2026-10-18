package harnessnames_test

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/registry"
	"github.com/Goldziher/ai-rulez/v5/internal/harnessnames"
)

func TestEveryHarnessPresetHasATableAndNoTableIsOrphaned(t *testing.T) {
	// Arrange
	registered := map[string]bool{}
	for _, n := range registry.Default().Names() {
		registered[n] = true
	}
	// Act / Assert
	for n := range registered {
		if _, ok := harnessnames.For(n); !ok && n != "mcp" && n != "okf" && n != "llms-txt" {
			t.Errorf("preset %q has no harness name table (add an empty one with completeness = none)", n)
		}
	}
	for _, p := range harnessnames.Presets() {
		if !registered[p] {
			t.Errorf("table %q is not a registered preset", p)
		}
	}
}
