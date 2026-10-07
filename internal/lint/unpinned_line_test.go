package lint

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnpinnedRemote_PointsAtTheIncludeWhateverTheQuoting(t *testing.T) {
	tests := []struct {
		name  string
		quote string
	}{
		{"basic string", `"`},
		{"literal string", `'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			q := tt.quote
			cfg := baseConfig + "\n[[skill_sources]]\nname = " + q + "shared" + q + "\nurl = " + q + "https://example.com/org/rules.git" + q + "\nref = " + q + "main" + q + "\n"
			writeFiles(t, root, map[string]string{
				".ai-rulez/config.toml":       cfg,
				".ai-rulez/skills/a/SKILL.md": "---\nname: a\ndescription: Plain static skill. Use when testing source pinning findings.\n---\nBody.\n",
			})
			gitAdd(t, root)
			wantLine := 1 + strings.Count(cfg[:strings.Index(cfg, "name = "+q+"shared")], "\n")

			// Act
			findings := lintDir(t, root)

			// Assert
			var line []int
			for _, f := range findings {
				if f.Code == CodeUnpinnedRemote && strings.Contains(f.Message, `"shared"`) {
					line = append(line, f.Line)
				}
			}
			require.Len(t, line, 1, "%v", findings)
			assert.Equal(t, wantLine, line[0])
		})
	}
}
