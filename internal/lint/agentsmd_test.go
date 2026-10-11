package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shimClaudeMD is the generated CLAUDE.md: a banner followed by the @AGENTS.md
// import and nothing else.
const shimClaudeMD = "<!--\n🤖 AI-RULEZ :: GENERATED FILE — DO NOT EDIT\nProject: t\n-->\n\n@AGENTS.md\n"

func TestClaudeMDShadow_OffByDefault(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml": baseConfig,
		"AGENTS.md":             "# Agents\n",
		"CLAUDE.md":             "# Hand written\n",
	})
	gitAdd(t, root)

	assert.Zero(t, countCode(lintDir(t, root), CodeClaudeMDShadow), "AR9Q0 is off without [lint] agents_md_only")
}

func TestClaudeMDShadow_FiresForStrayClaudeMD(t *testing.T) {
	tests := []struct {
		name   string
		files  map[string]string
		want   []string
		absent []string
	}{
		{name: "a root CLAUDE.md", files: map[string]string{"CLAUDE.md": "# Hand written\n"}, want: []string{"CLAUDE.md"}},
		{name: "a .claude/CLAUDE.md", files: map[string]string{".claude/CLAUDE.md": "# Hand written\n"}, want: []string{".claude/CLAUDE.md"}},
		{name: "a developer-local CLAUDE.local.md", files: map[string]string{"CLAUDE.local.md": "# Mine\n"}, want: []string{"CLAUDE.local.md"}},
		{name: "the generated shim", files: map[string]string{"CLAUDE.md": shimClaudeMD}, absent: []string{"CLAUDE.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			files := map[string]string{
				".ai-rulez/config.toml": baseConfig + "\n[lint]\nagents_md_only = true\n",
				"AGENTS.md":             "# Agents\n",
			}
			for k, v := range tt.files {
				files[k] = v
			}
			writeFiles(t, root, files)
			gitAdd(t, root)

			// Act
			fs := lintDir(t, root)

			// Assert
			for _, suffix := range tt.want {
				assert.True(t, has(fs, CodeClaudeMDShadow, suffix, 0), "%s should fire: %v", suffix, fs)
			}
			for _, suffix := range tt.absent {
				assert.False(t, has(fs, CodeClaudeMDShadow, suffix, 0), "%s should not fire: %v", suffix, fs)
			}
		})
	}
}

func TestClaudeMDShadow_MessageNamesTheShadowedAgentsMD(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":         baseConfig + "\n[lint]\nagents_md_only = true\n",
		"products/em/backend/AGENTS.md": "# Backend\n",
		"products/em/backend/CLAUDE.md": "# Hand written\n",
	})
	gitAdd(t, root)

	fs := lintDir(t, root)
	require.Equal(t, 1, countCode(fs, CodeClaudeMDShadow), "%v", fs)
	assert.Contains(t, messages(fs, CodeClaudeMDShadow), "products/em/backend/AGENTS.md", "the message names the nearest AGENTS.md")
}

func TestClaudeMDShadow_HonorsTheAllowList(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":         baseConfig + "\n[lint]\nagents_md_only = true\nagents_md_only_allow = [\"products/em/backend/**\"]\n",
		"AGENTS.md":                     "# Agents\n",
		"CLAUDE.md":                     "# Hand written\n",
		"products/em/backend/AGENTS.md": "# Backend\n",
		"products/em/backend/CLAUDE.md": "# Intentional island\n",
	})
	gitAdd(t, root)

	fs := lintDir(t, root)
	assert.True(t, has(fs, CodeClaudeMDShadow, "CLAUDE.md", 0), "the unallowed root CLAUDE.md fires: %v", fs)
	assert.False(t, has(fs, CodeClaudeMDShadow, "products/em/backend/CLAUDE.md", 0), "the allowed island does not fire")
	assert.Equal(t, 1, countCode(fs, CodeClaudeMDShadow))
}

// A --recursive run lints each nested root; the top-level run must not report
// the files a nested root owns.
func TestClaudeMDShadow_SkipsNestedRoots(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".ai-rulez/config.toml":     baseConfig + "\n[lint]\nagents_md_only = true\n",
		"AGENTS.md":                 "# Root agents\n",
		"svc/.ai-rulez/config.toml": baseConfig + "\n[lint]\nagents_md_only = true\n",
		"svc/AGENTS.md":             "# Svc agents\n",
		"svc/CLAUDE.md":             "# Hand written\n",
	})
	gitAdd(t, root)

	fs := lintDir(t, root)
	assert.False(t, has(fs, CodeClaudeMDShadow, "svc/CLAUDE.md", 0), "a nested root owns its own files: %v", fs)
}

func TestIsAgentsMDShim(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"the generated banner and import", shimClaudeMD, true},
		{"a bare import", "@AGENTS.md\n", true},
		{"a bare import with surrounding blank lines", "\n\n  @AGENTS.md  \n\n", true},
		{"a hand-written file", "# Project\n\nUse Go.\n", false},
		{"an import plus other content", "@AGENTS.md\n\nUse Go.\n", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isAgentsMDShim(tt.in))
		})
	}
}
