package exposure

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reportWithBudget is the start-of-session report of root checked against budget.
func reportWithBudget(t *testing.T, root string, budget int) *Report {
	t.Helper()
	return NewReport(build(t, root), ReportOptions{ContextWindow: 200000, StartBudget: budget})
}

func TestBuild_ImportIsCountedInTheStartBudget(t *testing.T) {
	// Arrange: the reproduction of #532, CLAUDE.md importing a large AGENTS.md.
	root := writeRepo(t, map[string]string{
		"CLAUDE.md": "@AGENTS.md\n",
		"AGENTS.md": strings.Repeat("Always follow these instructions.\n", 100),
	})

	// Act
	inv := build(t, root)
	rep := NewReport(inv, ReportOptions{ContextWindow: 200000, StartBudget: 50})

	// Assert: the imported body is attributed to CLAUDE.md and counted.
	claude := named(t, inv, KindClaudeMD, "CLAUDE.md")
	assert.Equal(t, []string{"AGENTS.md"}, importPaths(claude))
	assert.Positive(t, claude.ImportTokens)
	assert.Equal(t, EstimateTokens(len("@AGENTS.md\n"))+claude.ImportTokens, claude.Tokens)
	assert.Greater(t, rep.StartTokens, 50)
	assert.True(t, rep.OverStartBudget())
	assert.Empty(t, rep.StartImportsUnmodeled)
}

func TestBuild_SmallImportFitsTheBudget(t *testing.T) {
	// Arrange: the positive control, the same shape with a small import.
	root := writeRepo(t, map[string]string{
		"CLAUDE.md": "@AGENTS.md\n",
		"AGENTS.md": "Short instructions.\n",
	})

	// Act
	rep := reportWithBudget(t, root, 50)

	// Assert
	assert.LessOrEqual(t, rep.StartTokens, 50)
	assert.False(t, rep.OverStartBudget())
	assert.Len(t, rep.StartImportsUnmodeled, 0)
}

func TestBuild_ImportCycleTerminatesAndCountsEachFileOnce(t *testing.T) {
	// Arrange: a imports b imports a.
	root := writeRepo(t, map[string]string{
		"CLAUDE.md": "@a.md\n",
		"a.md":      "@b.md\nbody a\n",
		"b.md":      "@a.md\nbody b\n",
	})

	// Act
	inv := build(t, root)

	// Assert: no hang, and the cycle neither duplicates a nor fails closed.
	claude := named(t, inv, KindClaudeMD, "CLAUDE.md")
	assert.Equal(t, []string{"a.md", "b.md"}, importPaths(claude))
	assert.Equal(t, EstimateTokens(len("@a.md\nbody a\n"))+EstimateTokens(len("@a.md\nbody b\n")), claude.ImportTokens)
	assert.Empty(t, claude.ImportProblems)
}

func TestBuild_ImportLeavingTheRootFailsClosed(t *testing.T) {
	// Arrange: three directives the inventory must not follow.
	outside := filepath.Join(t.TempDir(), "outside.md")
	require.NoError(t, os.WriteFile(outside, []byte("outside\n"), 0o600))
	root := writeRepo(t, map[string]string{
		"CLAUDE.md": "@../outside.md\n@/etc/passwd.md\n@~/secret.md\n",
	})

	// Act
	inv := build(t, root)
	rep := NewReport(inv, ReportOptions{ContextWindow: 200000, StartBudget: 500})

	// Assert
	claude := named(t, inv, KindClaudeMD, "CLAUDE.md")
	assert.Empty(t, claude.Imports)
	require.Len(t, claude.ImportProblems, 3)
	assert.Len(t, rep.StartImportsUnmodeled, 3)
	assert.Less(t, rep.StartTokens, 500)
	assert.True(t, rep.OverStartBudget(), "a configured budget fails closed on an unmodeled import")
}

func TestResolveImportPath(t *testing.T) {
	tests := []struct {
		name, from, ref, wantRel, wantReason string
	}{
		{name: "beside the file", from: "CLAUDE.md", ref: "AGENTS.md", wantRel: "AGENTS.md"},
		{name: "below the file", from: "services/CLAUDE.md", ref: "docs/style.md", wantRel: "services/docs/style.md"},
		{name: "a parent inside the root", from: "services/CLAUDE.md", ref: "../AGENTS.md", wantRel: "AGENTS.md"},
		{name: "leaves the root", from: "CLAUDE.md", ref: "../AGENTS.md", wantReason: "outside the repository root"},
		{name: "absolute", from: "CLAUDE.md", ref: "/etc/passwd.md", wantReason: "absolute or home path"},
		{name: "home", from: "CLAUDE.md", ref: "~/.claude/CLAUDE.md", wantReason: "absolute or home path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel, reason := resolveImportPath(tt.from, tt.ref)

			assert.Equal(t, tt.wantRel, rel)
			assert.Contains(t, reason, tt.wantReason)
		})
	}
}

func TestImportRefs(t *testing.T) {
	text := "Use @AGENTS.md and mail bob@example.com.\n`@inline.md`\n```\n@fenced.md\n```\n" +
		"@./nested/rule.md, @https://example.com/x.md\n"
	refs := importRefs(text)
	got := make([]string, 0, len(refs))
	for _, r := range refs {
		got = append(got, r.path)
	}
	assert.Equal(t, []string{"AGENTS.md", "./nested/rule.md"}, got)
	assert.Equal(t, 1, refs[0].line)
}

func importPaths(a Artifact) []string {
	out := make([]string, 0, len(a.Imports))
	for _, im := range a.Imports {
		out = append(out, im.Path)
	}
	return out
}
