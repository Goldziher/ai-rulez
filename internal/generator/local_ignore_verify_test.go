package generator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const driftSharedIgnoring = `version = "5.0"
name = "shared-project"
presets = ["claude"]
`

// checkIgnored reports whether git ignores rel in the project.
func (p *driftProject) checkIgnored(t *testing.T, rel string) bool {
	t.Helper()
	cmd := exec.Command("git", "-C", p.base, "check-ignore", "--no-index", "-q", rel) //nolint:gosec // test
	return cmd.Run() == nil
}

func (p *driftProject) writeFile(t *testing.T, rel, content string) {
	t.Helper()
	path := filepath.Join(p.base, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestGenerate_UserNegationOfAMachineLocalOutputFailsClosed(t *testing.T) {
	// Arrange: the overlay adds codex, whose AGENTS.md is excluded per clone, and
	// a .gitignore rule un-ignores it (a .gitignore beats .git/info/exclude).
	p := newDriftProject(t, driftSharedIgnoring)
	p.git(t, "init", "-q")
	p.writeFile(t, ".gitignore", "!AGENTS.md\n")
	p.overlay(t, "presets = [\"codex\"]\n")

	// Act
	err := NewGenerator(p.load(t)).Generate("")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not git-ignored")
	assert.Contains(t, err.Error(), "AGENTS.md")
	assert.False(t, p.exists("AGENTS.md"), "the local file is not written while it could be committed")
}

func TestGenerate_UserRuleThatOnlyLooksLikeCoverageDoesNotHideLocalRules(t *testing.T) {
	// Arrange: "x.*" matched the old probe path for ".claude/rules/*.local.*".
	p := newDriftProject(t, driftSharedIgnoring)
	p.git(t, "init", "-q")
	p.writeFile(t, ".gitignore", "x.*\n")
	p.writeFile(t, ".ai-rulez/local/rules/mine.md", "---\npriority: low\n---\n\nPrivate.\n")

	// Act
	err := NewGenerator(p.load(t)).Generate("")

	// Assert
	require.NoError(t, err)
	require.True(t, p.exists(".claude/rules/mine.local.md"))
	assert.True(t, p.checkIgnored(t, ".claude/rules/mine.local.md"))
}

func TestGenerate_SymlinkedRootGitignoreIsNotWrittenThrough(t *testing.T) {
	// Arrange
	p := newDriftProject(t, driftSharedIgnoring)
	p.git(t, "init", "-q")
	target := filepath.Join(t.TempDir(), "shared-gitignore")
	require.NoError(t, os.WriteFile(target, []byte("# kept\n"), 0o644))
	testutil.SymlinkOrSkip(t, target, filepath.Join(p.base, ".gitignore"))
	p.writeFile(t, ".ai-rulez/local/rules/mine.md", "---\npriority: low\n---\n\nPrivate.\n")

	// Act
	err := NewGenerator(p.load(t)).Generate("")

	// Assert
	require.NoError(t, err)
	data, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, "# kept\n", string(data), "the link target is not modified")
	assert.True(t, p.checkIgnored(t, ".claude/rules/mine.local.md"))
	assert.True(t, p.checkIgnored(t, ".ai-rulez/config.local.toml"))
	assert.True(t, p.checkIgnored(t, ".ai-rulez/local/rules/mine.md"))
}

func TestGenerate_RefusedRunStillIgnoresTheOverlay(t *testing.T) {
	// Arrange: a shared gemini settings file would carry an overlay secret.
	p := newDriftProject(t, driftShared+"agents_md = true\n")
	p.writeFile(t, ".ai-rulez/config.toml", strings.Replace(driftShared, `presets = ["claude"]`, `presets = ["gemini"]`, 1)+"agents_md = true\n")
	p.git(t, "init", "-q")
	p.overlay(t, "[[mcp_servers]]\nname = \"loc\"\ncommand = \"x\"\n[mcp_servers.env]\nAPI_TOKEN = \"supersecretvalue123\"\n")

	// Act
	err := NewGenerator(p.load(t)).Generate("")

	// Assert
	require.Error(t, err)
	assert.True(t, p.checkIgnored(t, ".ai-rulez/config.local.toml"), "the overlay is ignored although the run was refused")
	assert.False(t, p.exists(".gemini/settings.json"))
	assert.Contains(t, hintOf(err), "shared with your team")
	assert.NotContains(t, err.Error(), "supersecretvalue123")
}

func TestGenerate_NoLocalKeepsTheLocalIgnoreEntries(t *testing.T) {
	// Arrange: claude + cursor with a local rule and an overlay preset.
	p := newDriftProject(t, strings.Replace(driftShared, `presets = ["claude"]`, `presets = ["claude", "cursor"]`, 1))
	p.git(t, "init", "-q")
	p.overlay(t, "presets = [\"devin\"]\n")
	p.writeFile(t, ".ai-rulez/local/rules/mine.md", "---\npriority: low\n---\n\nPrivate.\n")
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))
	withLocal := p.read(t, ".gitignore")
	require.Contains(t, withLocal, ".claude/rules/*.local.*")

	// Act
	require.NoError(t, NewGenerator(p.load(t, config.WithoutLocal())).Generate(""))

	// Assert
	after := p.read(t, ".gitignore")
	for _, line := range []string{".claude/rules/*.local.*", ".cursor/rules/*.local.*", ".ai-rulez/local/", ".ai-rulez/config.local.*"} {
		if strings.Contains(withLocal, line) {
			assert.Contains(t, after, line, "--no-local keeps the entry for existing local files")
		}
	}
	assert.True(t, p.checkIgnored(t, ".claude/rules/mine.local.md"))
}

func TestDropCoveredPatterns(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"child of a directory", []string{".xum/", ".xum/mcp.jsonc", "AGENTS.md"}, []string{".xum/", "AGENTS.md"}},
		{"sibling with the same prefix stays", []string{".xum/", ".xum2/file"}, []string{".xum/", ".xum2/file"}},
		{"glob directories do not cover", []string{".claude/*/", ".claude/x"}, []string{".claude/*/", ".claude/x"}},
		{"nothing to drop", []string{"a", "b"}, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, dropCoveredPatterns(tt.in))
		})
	}
}

func TestGenerate_ManagedGitignoreNarrowsXumDir(t *testing.T) {
	// Arrange
	p := newDriftProject(t, strings.Replace(strings.Replace(driftShared, `presets = ["claude"]`, `presets = ["xum"]`, 1),
		"gitignore = false", "gitignore = true", 1))

	// Act
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))

	// Assert: the shared .xum/ directory is never ignored whole, so a hand-authored
	// file in it stays visible to git; only the generated parts are listed.
	lines := strings.Split(p.read(t, ".gitignore"), "\n")
	assert.NotContains(t, lines, ".xum/")
	assert.Contains(t, lines, ".xum/skills/")
}

// hintOf returns the hint carried by an error, or "".
func hintOf(err error) string {
	if oopsErr, ok := oops.AsOops(err); ok {
		return oopsErr.Hint()
	}
	return ""
}

func geminiLocalMCPProject(t *testing.T) *driftProject {
	t.Helper()
	p := newDriftProject(t, driftShared)
	p.writeFile(t, ".ai-rulez/config.toml",
		"version = \"5.0\"\nname = \"shared-project\"\npresets = [\"gemini\", \"claude\"]\nagents_md = true\n")
	p.git(t, "init", "-q")
	p.overlay(t, "[[mcp_servers]]\nname = \"loc\"\ncommand = \"x\"\n")
	return p
}

func TestGenerate_ManifestsAreStableAcrossRunsWithAnOverlayMCPServer(t *testing.T) {
	// Arrange
	p := geminiLocalMCPProject(t)
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))
	shared, local := p.read(t, ".ai-rulez/.generated-manifest.json"), p.read(t, ".ai-rulez/.generated-manifest.local.json")
	settings := p.read(t, ".gemini/settings.json")

	// Act
	require.NoError(t, NewGenerator(p.load(t)).Generate(""))

	// Assert
	assert.Equal(t, shared, p.read(t, ".ai-rulez/.generated-manifest.json"))
	assert.Equal(t, local, p.read(t, ".ai-rulez/.generated-manifest.local.json"))
	assert.Equal(t, settings, p.read(t, ".gemini/settings.json"))
}

func TestDryRun_NamesDriftThatIsGitIgnoredAsAllowed(t *testing.T) {
	// Arrange
	p := geminiLocalMCPProject(t)

	// Act
	lines, err := NewGenerator(p.load(t)).DryRun("")

	// Assert
	require.NoError(t, err)
	assert.Contains(t, lines, "drift: .gemini/settings.json")
	assert.Contains(t, lines, "allowed: .gemini/settings.json (drift, but git-ignored)")
	assert.False(t, containsPrefix(lines, "blocked:"), "%v", lines)
}
