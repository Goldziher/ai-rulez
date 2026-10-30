package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestGitignoreExcludes(t *testing.T) {
	tests := []struct {
		name    string
		entry   string
		pattern string
		want    bool
	}{
		{"exact file", "AGENTS.md", "AGENTS.md", true},
		{"slash-less entry matches at any depth", "AGENTS.md", "svc/AGENTS.md", true},
		{"slash-less entry is not a prefix match", "AGENTS.md", "AGENTS.md.bak", false},
		{"double star reaches the root file", "**/AGENTS.md", "AGENTS.md", true},
		{"double star reaches nested files", "**/CLAUDE.md", "svc/web/CLAUDE.md", true},
		{"anchored entry stays at the root", ".claude/rules/r.md", "svc/.claude/rules/r.md", false},
		{"directory entry", ".claude/skills/", ".claude/skills/", true},
		{"directory entry covers a file below", ".claude/", ".claude/rules/r.md", true},
		{"directory entry does not match a file of that name", "skills/", "skills", false},
		{"glob inside a segment", ".claude/rules/*.md", ".claude/rules/go.md", true},
		{"glob does not cross directories", ".claude/*.md", ".claude/rules/go.md", false},
		{"leading dot slash is notation", "./.mcp.json", ".mcp.json", true},
		{"leading slash anchors", "/AGENTS.md", "AGENTS.md", true},
		{"leading slash does not reach nested files", "/AGENTS.md", "svc/AGENTS.md", false},
		{"leading slash on a directory stays at the root", "/skills/", "svc/skills/", false},
		{"unrelated", ".cursor/", ".claude/skills/", false},
		{"empty entry", "", "AGENTS.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, gitignoreExcludes(tt.entry, tt.pattern))
		})
	}
}

// newExcludeRepo is a git repo with the claude preset, a skill and the given
// extra top-level config lines (gitignore = true is always set).
func newExcludeRepo(t *testing.T, extra string) string {
	t.Helper()
	root := newPlainRepo(t, true)
	skill := filepath.Join(root, ".ai-rulez", "skills", "s", "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(skill), 0o755))
	require.NoError(t, os.WriteFile(skill, []byte("---\nname: s\ndescription: d\n---\n\nbody\n"), 0o644))
	cfgPath := filepath.Join(root, ".ai-rulez", "config.toml")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfgPath, append(data, extra...), 0o644))
	return root
}

func generateWithLog(t *testing.T, root string) (*Generator, *testutil.LogRecorder) {
	t.Helper()
	rec := &testutil.LogRecorder{}
	cfg, err := config.LoadConfig(context.Background(), root, config.WithHost(ambient.Host{Log: rec}))
	require.NoError(t, err)
	g := NewGenerator(cfg)
	require.NoError(t, g.Generate("default"))
	return g, rec
}

func TestGenerate_GitignoreExclude(t *testing.T) {
	tests := []struct {
		name       string
		exclude    string
		absent     []string // block lines that must not be there
		present    []string // block lines that stay managed
		wantWarned string   // a warning that must be logged
	}{
		{
			name:    "committed outputs stay out of the block",
			exclude: `gitignore_exclude = ["AGENTS.md", "CLAUDE.md", ".claude/skills/"]`,
			absent:  []string{"\nAGENTS.md\n", "\nCLAUDE.md\n", "\n.claude/skills/\n"},
			present: []string{".ai-rulez/.generated-manifest.local.json", ".ai-rulez/.generated-manifest.json", ".mcp.json"},
		},
		{
			name:    "nested globs",
			exclude: `gitignore_exclude = ["**/AGENTS.md", "**/CLAUDE.md"]`,
			absent:  []string{"\nAGENTS.md\n", "\nCLAUDE.md\n"},
			present: []string{".claude/skills/"},
		},
		{
			name:       "a machine-local entry cannot be excluded",
			exclude:    `gitignore_exclude = [".ai-rulez/.generated-manifest.local.json", "CLAUDE.md"]`,
			absent:     []string{"\nCLAUDE.md\n"},
			present:    []string{".ai-rulez/.generated-manifest.local.json"},
			wantWarned: "does not apply to machine-local or secret outputs",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := newExcludeRepo(t, tt.exclude+"\n")

			// Act
			_, rec := generateWithLog(t, root)
			first := readGitignore(t, root)
			g, _ := generateWithLog(t, root)
			second := readGitignore(t, root)
			left, err := g.UnignoredOutputs("default")

			// Assert
			require.NoError(t, err)
			assert.Equal(t, first, second, "a second generate writes the same .gitignore")
			for _, line := range tt.absent {
				assert.NotContains(t, first, line)
			}
			for _, line := range tt.present {
				assert.Contains(t, first, line)
			}
			assert.Empty(t, left, "doctor reports nothing for an excluded output")
			if tt.wantWarned != "" {
				assert.Condition(t, func() bool {
					return countContaining(rec.Level("WARN"), tt.wantWarned) == 1
				}, "%v", rec.Level("WARN"))
			}
		})
	}
}

func TestGenerate_WarnsWhenAHumanNegationConflictsWithAManagedLine(t *testing.T) {
	tests := []struct {
		name     string
		extra    string
		wantWarn bool
	}{
		{name: "negation of a managed line", wantWarn: true},
		{name: "the same path in gitignore_exclude is a deliberate choice", extra: "gitignore_exclude = [\"CLAUDE.md\"]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := newExcludeRepo(t, tt.extra)
			require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("!CLAUDE.md\n"), 0o644))

			// Act
			_, rec := generateWithLog(t, root)

			// Assert
			got := countContaining(rec.Level("WARN"), "un-ignores /CLAUDE.md")
			if tt.wantWarn {
				assert.Equal(t, 1, got, "%v", rec.Level("WARN"))
				assert.Condition(t, func() bool {
					return countContaining(rec.Level("WARN"), ".gitignore:1") == 1 && countContaining(rec.Level("WARN"), "gitignore_exclude") == 1
				}, "the warning names the negation's file:line and the fix: %v", rec.Level("WARN"))
				return
			}
			assert.Zero(t, got)
		})
	}
}

func TestGenerate_WarnsWhenTheManagedBlockHidesATrackedFile(t *testing.T) {
	// Arrange: CLAUDE.md is committed although the block lists it.
	root := newExcludeRepo(t, "")
	generateWithLog(t, root)
	require.NoError(t, gitutil.CommandNoContext(root, "add", "-f", "CLAUDE.md").Run())

	// Act
	_, rec := generateWithLog(t, root)

	// Assert
	warned := rec.Level("WARN")
	assert.Equal(t, 1, countContaining(warned, "Tracked files match the managed .gitignore block"), "%v", warned)
	assert.Equal(t, 1, countContaining(warned, "files=CLAUDE.md"), "%v", warned)
	assert.Equal(t, 1, countContaining(warned, "gitignore_exclude"), "the fix names the knob: %v", warned)
}

func TestTrackedBlockWarnings_NamesNestedFilesAnUnanchoredLineHides(t *testing.T) {
	// Arrange: a recursive block lists the root's AGENTS.md without an anchor, so
	// it also matches the nested root's committed file.
	root := t.TempDir()
	initGitRepo(t, root)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "svc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "svc", "AGENTS.md"), []byte("# svc\n"), 0o644))
	block := gitignore.BeginMarker + "\nAGENTS.md\nsvc/.claude/skills/\n" + gitignore.EndMarker + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte(block), 0o644))
	require.NoError(t, gitutil.CommandNoContext(root, "add", "-f", "svc/AGENTS.md").Run())
	rec := &testutil.LogRecorder{}

	// Act
	found := trackedBlockWarnings(context.Background(), gitutil.Git{}, root, block)
	warnTrackedByBlock(rec.Warn, found)

	// Assert
	assert.Equal(t, []trackedByBlock{{Path: "svc/AGENTS.md", Rule: "AGENTS.md"}}, found)
	warned := strings.Join(rec.Level("WARN"), "\n")
	assert.Contains(t, warned, "files=svc/AGENTS.md")
	assert.Contains(t, warned, `**/AGENTS.md`, "the unanchored line is called out")
}
