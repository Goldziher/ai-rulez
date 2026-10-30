package generator

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// rootOutputs lists the committed outputs of a root at dir.
func rootOutputs(dir string) []config.OutputFile {
	return []config.OutputFile{
		{Path: filepath.Join(dir, "CLAUDE.md")},
		{Path: filepath.Join(dir, ".claude", "skills", "demo", "SKILL.md")},
	}
}

// TestGitignoreSink_WritesOneBlockAtTheToplevel is the regression test for
// #316: a recursive run must collect every root's patterns into one block at
// the git toplevel, expressed relative to it, instead of writing a .gitignore
// beside every nested root.
func TestGitignoreSink_WritesOneBlockAtTheToplevel(t *testing.T) {
	// Arrange
	top := gitRepo(t, "")
	api := filepath.Join(top, "packages", "api")
	require.NoError(t, os.MkdirAll(api, 0o755))
	sink := NewGitignoreSink(top)

	rootGen := NewGenerator(&config.Config{BaseDir: top, Gitignore: gitignoreOn()})
	rootGen.SetGitignoreSink(sink, "")
	require.NoError(t, rootGen.updateGitignore(rootOutputs(top)))

	apiGen := NewGenerator(&config.Config{BaseDir: api, Gitignore: gitignoreOn()})
	apiGen.SetGitignoreSink(sink, "packages/api")
	require.NoError(t, apiGen.updateGitignore(rootOutputs(api)))

	// Act
	require.NoError(t, sink.Write(t.Context(), gitutil.Git{}.WithLog(logger.Discard())))

	// Assert: one block at the toplevel, patterns relative to each root.
	inside := gitignore.PatternsInsideFence(readGitignore(t, top))
	assert.Contains(t, inside, "/CLAUDE.md")
	assert.Contains(t, inside, ".claude/skills/")
	assert.Contains(t, inside, "packages/api/CLAUDE.md")
	assert.Contains(t, inside, "packages/api/.claude/skills/")

	// Assert: the nested root, which had no .gitignore, gains none.
	_, err := os.Stat(filepath.Join(api, ".gitignore"))
	assert.True(t, os.IsNotExist(err), "a nested root without .gitignore must not gain one")

	// Assert: git ignores the nested outputs through the single toplevel block.
	for _, rel := range []string{"packages/api/CLAUDE.md", "packages/api/.claude/skills/demo/SKILL.md"} {
		cmd := exec.Command("git", "-C", top, "check-ignore", "--no-index", "-q", rel) //nolint:gosec // test
		assert.NoError(t, cmd.Run(), "%s must be ignored", rel)
	}
}

// TestGitignoreSink_DropsPatternsOfARootThatNoLongerEmitsThem pins that the
// consolidation removes what an earlier recursive run left, rather than
// growing the block forever.
func TestGitignoreSink_DropsPatternsOfARootThatNoLongerEmitsThem(t *testing.T) {
	// Arrange: a first run records the nested root's patterns.
	top := gitRepo(t, "")
	api := filepath.Join(top, "packages", "api")
	require.NoError(t, os.MkdirAll(api, 0o755))

	first := NewGitignoreSink(top)
	firstGen := NewGenerator(&config.Config{BaseDir: api, Gitignore: gitignoreOn()})
	firstGen.SetGitignoreSink(first, "packages/api")
	require.NoError(t, firstGen.updateGitignore(rootOutputs(api)))
	require.NoError(t, first.Write(t.Context(), gitutil.Git{}.WithLog(logger.Discard())))
	require.Contains(t, readGitignore(t, top), "packages/api/.claude/skills/")

	// Act: a second run where the root emits no committed outputs.
	second := NewGitignoreSink(top)
	secondGen := NewGenerator(&config.Config{BaseDir: api, Gitignore: gitignoreOn()})
	secondGen.SetGitignoreSink(second, "packages/api")
	require.NoError(t, secondGen.updateGitignore(nil))
	require.NoError(t, second.Write(t.Context(), gitutil.Git{}.WithLog(logger.Discard())))

	// Assert: the stale patterns are gone.
	assert.NotContains(t, readGitignore(t, top), "packages/api/.claude/skills/")
	assert.NotContains(t, readGitignore(t, top), "packages/api/CLAUDE.md")
}

// TestGitignoreSink_CommittedOutputsStayOutOfTheBlock pins #316(3): a root
// whose gitignore is off commits its outputs, so nothing of it may reach the
// block, and no toplevel .gitignore is created for it.
func TestGitignoreSink_CommittedOutputsStayOutOfTheBlock(t *testing.T) {
	// Arrange
	top := gitRepo(t, "")
	api := filepath.Join(top, "packages", "api")
	require.NoError(t, os.MkdirAll(api, 0o755))
	sink := NewGitignoreSink(top)
	gen := NewGenerator(&config.Config{BaseDir: api}) // gitignore off
	gen.SetGitignoreSink(sink, "packages/api")

	// Act
	require.NoError(t, gen.updateGitignore(rootOutputs(api)))
	require.NoError(t, sink.Write(t.Context(), gitutil.Git{}.WithLog(logger.Discard())))

	// Assert: nothing was listed and no file was created.
	content := readGitignore(t, top)
	assert.NotContains(t, content, "CLAUDE.md")
	assert.NotContains(t, content, ".claude/skills/")
	assert.NotContains(t, content, gitignore.BeginMarker)
}

// TestGitignoreSink_CleansUpLeftoverNestedBlock pins that a nested root with
// gitignore off removes the block the buggy recursive behavior (#316) left in
// its own .gitignore, without touching the shared toplevel block.
func TestGitignoreSink_CleansUpLeftoverNestedBlock(t *testing.T) {
	// Arrange: a nested root holds a stale managed block from an earlier run.
	top := gitRepo(t, "")
	api := filepath.Join(top, "packages", "api")
	require.NoError(t, os.MkdirAll(api, 0o755))
	stale := gitignore.BeginMarker + "\nCLAUDE.md\n" + gitignore.EndMarker + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(api, ".gitignore"), []byte(stale), 0o644))

	sink := NewGitignoreSink(top)
	gen := NewGenerator(&config.Config{BaseDir: api}) // gitignore off
	gen.SetGitignoreSink(sink, "packages/api")

	// Act
	gen.finishGitignore(nil, false)

	// Assert: the nested block is gone and the toplevel was left alone.
	assert.NotContains(t, readGitignore(t, api), gitignore.BeginMarker)
	assert.NotContains(t, readGitignore(t, top), gitignore.BeginMarker)
}

// TestJoinGitignorePrefix pins how a root's patterns are re-anchored at the
// toplevel.
func TestJoinGitignorePrefix(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		pattern string
		want    string
	}{
		{"toplevel root", "", ".claude/skills/", ".claude/skills/"},
		{"dot prefix is the toplevel root", ".", "CLAUDE.md", "CLAUDE.md"},
		{"nested root", "packages/api", ".claude/skills/", "packages/api/.claude/skills/"},
		{"nested root file", "packages/api", "CLAUDE.md", "packages/api/CLAUDE.md"},
		{"anchored pattern keeps its meaning", "packages/api", "/x.md", "packages/api/x.md"},
		{"blank pattern is dropped", "packages/api", "  ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, joinGitignorePrefix(tt.prefix, tt.pattern))
		})
	}
}
