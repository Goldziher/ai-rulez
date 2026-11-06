package exposure

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/repoindex"
)

func TestLeaks_ImportedTextIsScanned(t *testing.T) {
	// Arrange: the shape of #536, a CLAUDE.md that is only `@AGENTS.md`, with a leaked term in AGENTS.md.
	root := writeRepo(t, map[string]string{
		"CLAUDE.md": "@AGENTS.md\n",
		"AGENTS.md": "# Guide\n\nRun the ranker before a release.\n",
	})

	// Act
	leaks := Leaks(build(t, root).Start(), leakMap(t))

	// Assert: reported against the imported file and its own line.
	require.Len(t, leaks, 1)
	assert.Equal(t, LeakViolation, leaks[0].Severity)
	assert.Equal(t, "search", leaks[0].Area)
	assert.Equal(t, "ranker", leaks[0].Term)
	assert.Equal(t, "AGENTS.md", leaks[0].Path)
	assert.Equal(t, 3, leaks[0].Line)
	assert.Equal(t, KindClaudeMD, leaks[0].Kind)
	assert.Equal(t, "CLAUDE.md", leaks[0].Name)
}

func TestLeaks_CleanImportLeaksNothing(t *testing.T) {
	// Arrange: the positive control, the same shape with no area term.
	root := writeRepo(t, map[string]string{
		"CLAUDE.md": "@AGENTS.md\n",
		"AGENTS.md": "Run the tests before a release.\n",
	})

	// Act and assert
	assert.Empty(t, Leaks(build(t, root).Start(), leakMap(t)))
}

func TestLeaks_TransitiveImportAndSharedImportAreReportedOnce(t *testing.T) {
	// Arrange: CLAUDE.md imports a and b, both import c, which leaks.
	root := writeRepo(t, map[string]string{
		"CLAUDE.md": "@a.md\n@b.md\n",
		"a.md":      "@c.md\n",
		"b.md":      "@c.md\n",
		"c.md":      "the ranker\n",
	})

	// Act
	leaks := Leaks(build(t, root).Start(), leakMap(t))

	// Assert
	require.Len(t, leaks, 1)
	assert.Equal(t, "c.md", leaks[0].Path)
}

func TestLeaks_ImportOfAStartArtifactIsNotScannedTwice(t *testing.T) {
	// Arrange: an unconditional rule, itself a start-of-session artifact, that CLAUDE.md also imports.
	root := writeRepo(t, map[string]string{
		"CLAUDE.md":          "@.claude/rules/r.md\n",
		".claude/rules/r.md": "the ranker\n",
	})

	// Act
	leaks := Leaks(build(t, root).Start(), leakMap(t))

	// Assert
	require.Len(t, leaks, 1)
	assert.Equal(t, ".claude/rules/r.md", leaks[0].Path)
}

func TestLeaks_ImportInsideCodeIsNotScanned(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"CLAUDE.md": "`@AGENTS.md`\n",
		"AGENTS.md": "the ranker\n",
	})

	assert.Empty(t, Leaks(build(t, root).Start(), leakMap(t)))
}

func TestImportTextIsNotSerialized(t *testing.T) {
	root := writeRepo(t, map[string]string{"CLAUDE.md": "@AGENTS.md\n", "AGENTS.md": "secret body\n"})
	inv := build(t, root)

	data, err := json.Marshal(inv.WithoutText())

	require.NoError(t, err)
	assert.NotContains(t, string(data), "secret body")
}

func TestUncheckedArtifacts_UnreadImportsAreListed(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  Unchecked
	}{
		{
			name:  "an import over the read limit",
			files: map[string]string{"CLAUDE.md": "@big.md\n", "big.md": strings.Repeat("x", repoindex.MaxFileBytes+1)},
			want:  Unchecked{Path: "big.md", Kind: KindClaudeMD, Name: "CLAUDE.md", Reason: "imported by CLAUDE.md: the file is over the read limit"},
		},
		{
			name:  "an import that leaves the root",
			files: map[string]string{"CLAUDE.md": "@../x.md\n"},
			want:  Unchecked{Path: "../x.md", Kind: KindClaudeMD, Name: "CLAUDE.md", Reason: "imported by CLAUDE.md: the path resolves outside the repository root"},
		},
		{
			name:  "a home import",
			files: map[string]string{"CLAUDE.md": "@~/x.md\n"},
			want:  Unchecked{Path: "~/x.md", Kind: KindClaudeMD, Name: "CLAUDE.md", Reason: "imported by CLAUDE.md: an absolute or home path is not modeled"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			start := build(t, writeRepo(t, tt.files)).Start()

			// Act
			got := UncheckedArtifacts(start)

			// Assert
			assert.Equal(t, []Unchecked{tt.want}, got)
		})
	}
}

func TestUncheckedArtifacts_ReadImportsAreNotListed(t *testing.T) {
	start := build(t, writeRepo(t, map[string]string{"CLAUDE.md": "@AGENTS.md\n@missing.md\n", "AGENTS.md": "text\n"})).Start()

	assert.Empty(t, UncheckedArtifacts(start))
}

func TestLeaks_ImportedByNamesTheImporter(t *testing.T) {
	root := writeRepo(t, map[string]string{"CLAUDE.md": "@AGENTS.md\nthe ranker\n", "AGENTS.md": "the ranker\n"})

	leaks := Leaks(build(t, root).Start(), leakMap(t))

	require.Len(t, leaks, 2)
	assert.Empty(t, leaks[0].ImportedBy)
	assert.Equal(t, "AGENTS.md", leaks[1].Path)
	assert.Equal(t, []string{"CLAUDE.md"}, leaks[1].ImportedBy)
}

func TestLeaks_ImportedByListsEveryImporterAndTheChain(t *testing.T) {
	// Arrange: CLAUDE.md reaches c.md through a.md and m.md; an unconditional rule imports c.md directly.
	root := writeRepo(t, map[string]string{
		"CLAUDE.md":          "@a.md\n",
		"a.md":               "@m.md\n",
		"m.md":               "@c.md\n",
		"c.md":               "the ranker\n",
		".claude/rules/r.md": "@../../c.md\n",
	})

	// Act
	leaks := Leaks(build(t, root).Start(), leakMap(t))

	// Assert: both importers, in inventory order, and every file on the way to the leak.
	require.Len(t, leaks, 1)
	assert.Equal(t, "c.md", leaks[0].Path)
	assert.Equal(t, []string{"CLAUDE.md", ".claude/rules/r.md"}, leaks[0].ImportedBy)
	assert.Equal(t, KindClaudeMD, leaks[0].Kind)
	assert.Equal(t, "CLAUDE.md", leaks[0].Name)
	assert.Subset(t, leaks[0].ImportChain, []string{"CLAUDE.md", "a.md", "m.md", "c.md", ".claude/rules/r.md"})
}
