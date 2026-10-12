package templates_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/templates"
)

// A nested root's header must name its own config, relative to the outermost
// root, not the repository-root config (#361).
func TestHeader_NestedRootSourceIsRepoRelative(t *testing.T) {
	t.Parallel()

	for _, style := range []string{"detailed", "compact", "minimal"} {
		t.Run(style, func(t *testing.T) {
			t.Parallel()
			data := createTemplateData("N", &config.Config{
				Name:          "N",
				ConfigDirName: ".ai-rulez",
				RootRel:       "products/em/webapps",
				Header:        &config.HeaderConfig{Style: style},
			})
			header := templates.GenerateHeader(data)
			assert.Contains(t, header, "Source: products/em/webapps/.ai-rulez/config.toml")
			assert.NotContains(t, header, "Source: .ai-rulez/")
		})
	}
}
