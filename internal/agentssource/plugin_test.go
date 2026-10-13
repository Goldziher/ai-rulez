package agentssource_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/generator"
)

const handMarketplace = "{\"name\":\"hand-written\"}\n"

func TestGeneratePluginNeverWritesUnderTheAgentsSource(t *testing.T) {
	for _, layout := range layouts {
		t.Run(layout.name, func(t *testing.T) {
			// Arrange: a plugin whose Codex marketplace index would land in .agents/plugins
			body := layout.cfgBody + "\n[plugin]\nname = \"demo\"\nversion = \"1.0.0\"\nruntimes = [\"codex\"]\n\n[plugin.codex]\nmarketplace = true\n"
			root := setup(t, layout.cfgPath, body)
			write(t, root, ".agents/plugins/marketplace.json", handMarketplace)

			// Act
			err := generator.NewGenerator(load(t, root)).GeneratePlugin("")

			// Assert
			require.NoError(t, err)
			data, readErr := os.ReadFile(filepath.Join(root, ".agents", "plugins", "marketplace.json"))
			require.NoError(t, readErr)
			assert.Equal(t, handMarketplace, string(data), "the hand-written index is the project's own file")
			assert.NotContains(t, keys(snapshot(t, root)), ".agents/plugins/demo")
		})
	}
}
