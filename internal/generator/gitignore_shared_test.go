package generator

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
)

// TestGitignorePattern_SharedDirsNeverBecomeDirPatterns pins that a directory
// users keep hand-authored files in (.config/<tool>/, .vscode/, ...) is never
// ignored as a whole: generated files in it are ignored by exact path.
func TestGitignorePattern_SharedDirsNeverBecomeDirPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		path   string
		isDir  bool
		expect string
	}{
		{"config tool file", ".config/tool/settings.json", false, ".config/tool/settings.json"},
		{"config root file", ".config/tool.json", false, ".config/tool.json"},
		{"config dir marker", ".config", true, ""},
		{"config tool dir marker", ".config/tool", true, ""},
		{"config nested dir is owned", ".config/tool/skills", true, ".config/tool/skills/"},
		{"vscode file", ".vscode/mcp.json", false, ".vscode/mcp.json"},
		{"vscode dir marker", ".vscode", true, ""},
		{"zed file", ".zed/settings.json", false, ".zed/settings.json"},
		{"idea file", ".idea/mcp.xml", false, ".idea/mcp.xml"},
		{"husky file", ".husky/pre-commit", false, ".husky/pre-commit"},
		{"github other file", ".github/workflows/x.yml", false, ".github/workflows/x.yml"},
		{"nested config file", "apps/web/.config/tool/settings.json", false, "apps/web/.config/tool/settings.json"},
		{"nested vscode dir marker", "apps/web/.vscode", true, ""},
		{"agents skills stay narrowed", ".agents/skills/x/SKILL.md", false, ".agents/skills/"},
		{"agents root file exact", ".agents/settings.json", false, ".agents/settings.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expect, gitignorePatternForOutput(nil, tt.path, tt.isDir))
		})
	}
}

func TestGitignoreHints_ExcludeSharedDirs(t *testing.T) {
	// Act
	_, dirs := providers.GitignoreHints()

	// Assert
	for _, shared := range []string{".vscode/", ".idea/", ".zed/", ".config/", ".github/", ".husky/"} {
		assert.NotContains(t, dirs, shared)
		assert.NotContains(t, gitignoreAssistantDirs(), shared)
	}
}

func TestGitignorePattern_SidecarFileIsExact(t *testing.T) {
	t.Parallel()

	// .pi/mcp.json is a spec sidecar: a file, never its parent dir.
	assert.Equal(t, ".pi/mcp.json", gitignorePatternForOutput(nil, ".pi/mcp.json", false))
}
