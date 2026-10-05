package providers

import (
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
)

// TestMCPHarnessesMatchSpecs keeps config.HarnessSupportsMCP (which decides where
// served skills are reachable) in step with the embedded provider specs: a spec
// that writes MCP configuration is an MCP harness.
func TestMCPHarnessesMatchSpecs(t *testing.T) {
	t.Parallel()
	for _, spec := range loadBuiltinSpecs() {
		hasMCP := false
		for _, sc := range spec.Sidecars {
			if sc != nil && IsMCPSidecarKind(sc.Kind) {
				hasMCP = true
			}
		}
		if spec.Name == "mcp" {
			continue
		}
		assert.Equal(t, hasMCP, config.HarnessSupportsMCP(spec.Name), "preset %q: config.mcpHarnesses disagrees with its provider spec", spec.Name)
	}
}
