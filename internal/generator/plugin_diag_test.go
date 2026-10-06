package generator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Every plugin entry point issues its warnings through the run's own collector,
// never through the process default one that a nil collector stands for.
func TestPluginEntryPointsUseTheRunsCollector(t *testing.T) {
	calls := map[string]func(g *Generator){
		"GeneratePluginFiles": func(g *Generator) { _, _ = g.GeneratePluginFiles("") },
		"VerifyPlugin":        func(g *Generator) { _ = g.VerifyPlugin("") },
		"DryRunPlugin":        func(g *Generator) { _, _ = g.DryRunPlugin("") },
		"PluginVersionDrift":  func(g *Generator) { _, _ = g.PluginVersionDrift("") },
		"PluginFiles":         func(g *Generator) { _, _ = g.PluginFiles("") },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			// Arrange
			dir := newDomainsProject(t, driftTail("1.0.0"))
			g := loadDomainsProject(t, dir)
			g.config.Diag = nil

			// Act
			call(g)

			// Assert
			require.NotNil(t, g.config.Diag, "%s created no collector", name)
		})
	}
}
