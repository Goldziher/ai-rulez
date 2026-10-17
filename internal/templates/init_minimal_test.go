package templates

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestInitMinimalConfigTOML(t *testing.T) {
	tests := []struct {
		name        string
		project     string
		presets     []string
		wantName    string
		wantPresets []string
	}{
		{"named project and presets", "demo", []string{"claude", "cursor"}, "demo", []string{"claude", "cursor"}},
		{"no presets defaults to claude", "demo", nil, "demo", []string{"claude"}},
		{"blank name falls back to project", "   ", []string{"codex"}, "project", []string{"codex"}},
		{"a name with quotes is escaped", `my "app"`, []string{"claude"}, `my "app"`, []string{"claude"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			doc := InitMinimalConfigTOML(tt.project, tt.presets)
			cfg, err := config.DecodeTOMLConfig([]byte(doc), "ai-rulez.toml")

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, cfg.Name)
			var got []string
			for _, p := range cfg.Presets {
				got = append(got, p.BuiltIn)
			}
			assert.Equal(t, tt.wantPresets, got)
			assert.Empty(t, cfg.InstalledSkills, "the examples are comments")
			assert.True(t, strings.HasPrefix(doc, "#:schema https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/ai-rules.schema.json\n"))
			assert.Contains(t, doc, "version = \"5.0\"")
			assert.Contains(t, doc, "# [[installed_skills]]")
			assert.Contains(t, doc, "# [lint]")
		})
	}
}
