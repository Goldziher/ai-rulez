package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

func TestExposureLintOptions(t *testing.T) {
	withTable := &config.Config{Exposure: &config.ExposureConfig{}}
	tests := []struct {
		name string
		cfg  *config.Config
		sel  []string
		want int
	}{
		{"no configuration", nil, nil, 0},
		{"no [exposure] table", &config.Config{}, nil, 0},
		{"every analyzer", withTable, nil, 1},
		{"the exposure analyzer", withTable, []string{lint.AnalyzerExposure}, 1},
		{"the plugin analyzer, for AR964", withTable, []string{lint.AnalyzerPlugin}, 1},
		{"another analyzer", withTable, []string{lint.AnalyzerTraps}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Len(t, exposureLintOptions(t.Context(), tt.cfg, tt.sel), tt.want)
		})
	}
}

func TestExposureLintSourceStartExposure(t *testing.T) {
	// Arrange
	root := t.TempDir()
	for rel, body := range map[string]string{
		"CLAUDE.md":                         "# Repo\nRead the docs.\n",
		".claude/skills/deploy/SKILL.md":    "---\nname: deploy\ndescription: Deploys the service.\n---\nBody.\n",
		"svc/.claude/skills/local/SKILL.md": "---\nname: local\ndescription: Svc only.\n---\nBody.\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	src := exposureLintSource{ctx: t.Context(), cfg: &config.Config{BaseDir: root, Exposure: &config.ExposureConfig{ContextWindow: 1000}}, cache: &exposureCache{}}

	// Act
	start := src.StartExposure()

	// Assert
	require.NotNil(t, start)
	assert.Equal(t, 1000, start.ContextWindow)
	assert.Equal(t, 40, start.ListingBudgetChars)
	assert.Equal(t, 1, start.ListingSkills, "the svc skill loads on demand")
	assert.Equal(t, len("deploy: Deploys the service."), start.ListingChars)
	assert.Equal(t, 2, start.StartArtifacts, "the root CLAUDE.md and the deploy skill")
	assert.Positive(t, start.StartTokens)
	assert.Equal(t, []string{filepath.Join(root, "CLAUDE.md"), filepath.Join(root, ".claude", "skills", "deploy", "SKILL.md")}, start.StartPaths,
		"the files of the start-of-session artifacts, which AR9W3 and AR964 depend on")
	first, err := src.inventory(src.mode())
	require.NoError(t, err)
	second, err := src.inventory(src.mode())
	require.NoError(t, err)
	assert.Same(t, first, second, "the inventory is built once")
}

func TestExposureLintSourceWithoutAnInventory(t *testing.T) {
	src := exposureLintSource{ctx: t.Context(), cfg: &config.Config{BaseDir: t.TempDir(), Exposure: &config.ExposureConfig{Plugins: "some"}}}

	assert.Nil(t, src.StartExposure())
}
