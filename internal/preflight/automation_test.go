package preflight

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestIsAutomationPath(t *testing.T) {
	t.Parallel()
	tests := map[string]bool{
		".github/workflows/ci.yml": true, ".GitHub/Workflows/ci.yml": true, ".gitlab-ci.yml": true,
		"Makefile": true, "./Makefile": true, ".vscode/tasks.json": true, ".devcontainer/devcontainer.json": true,
		".husky/pre-commit": true, "package.json": true, "docs/AI_GUIDE.md": false, ".github/copilot-instructions.md": false,
		"docs/Makefile": false, ".vscode/extensions.json": false,
	}
	for p, want := range tests {
		assert.Equal(t, want, IsAutomationPath(p), p)
	}
}

func TestCollect_AnnouncesCustomPresetsWritingAutomationFiles(t *testing.T) {
	// Arrange
	cfg := &config.Config{Presets: []config.Preset{
		{Name: "ci", Type: config.PresetTypeMarkdown, Path: ".github/workflows/ci.yml", Template: "x"},
		{Name: "guide", Type: config.PresetTypeMarkdown, Path: "docs/GUIDE.md", Template: "x"},
	}}

	// Act
	items := Collect(cfg)

	// Assert
	var got []Item
	for _, it := range items {
		if it.Kind == "exec-file" {
			got = append(got, it)
		}
	}
	assert.Equal(t, []Item{{Kind: "exec-file", Command: "write .github/workflows/ci.yml", Where: []string{"ci"}}}, got)
}
