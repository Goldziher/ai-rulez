package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// rootComponentProjectConfig is a single-plugin configuration whose catalog lists
// the repository root, so its bundle root is the repository root.
const rootComponentProjectConfig = `version = "5.0"
name = "root-components"
presets = ["claude"]
agents_md = false

[plugin]
name = "p"
version = "1.0.0"
`

// verify --plugin reports AR9T0 (a single-plugin root that ships a default host
// component path) as a finding, without failing the run (#357).
func TestRootComponentWarningsReportsAShippedHostComponent(t *testing.T) {
	tests := []struct {
		name string
		root map[string]string
		want int
	}{
		{
			name: "a development .mcp.json at the plugin root",
			root: map[string]string{".mcp.json": "{}\n"},
			want: 1,
		},
		{
			name: "no default host component at the root",
			root: map[string]string{"README.md": "# readme\n"},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), rootComponentProjectConfig)
			writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nUse tabs.\n")
			for path, content := range tt.root {
				writeFile(t, filepath.Join(root, path), content)
			}
			chdir(t, root)
			cfg, err := loadConfigForCommand(t.Context(), nil)
			require.NoError(t, err)

			// Act
			got := rootComponentWarnings(cfg)

			// Assert
			require.Len(t, got, tt.want)
			if tt.want > 0 {
				assert.Equal(t, lint.CodeRootComponentShipped, got[0].Code)
				assert.Contains(t, got[0].Message, "Claude Code", "the finding names the host that loads the path")
				assert.Contains(t, got[0].Message, "content_root", "the finding offers the fix")
			}
		})
	}
}
