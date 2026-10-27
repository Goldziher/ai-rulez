package okfbridge

import (
	"os"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/okf"
)

func conceptWithKind(kind string) string {
	return "---\ntype: Concept\ntitle: Note\nx-ai-rulez:\n  kind: " + kind + "\n  id: note\n---\n\n# Note\n\nText.\n"
}

func TestKindFindings(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		body     string
		wantSev  okf.Severity
		wantText string
	}{
		{"unknown kind is an error", "context/note.md", conceptWithKind("bogus"), okf.SeverityError,
			`"bogus" is unknown (use rules, context, skills, agents, commands, checks or skill-resource)`},
		{"kind disagreeing with its directory is a warning", "context/note.md", conceptWithKind("skill"),
			okf.SeverityWarning, `"skill" disagrees with its directory context/`},
		{"mismatch inside a domain", "domains/web/rules/note.md", conceptWithKind("context"),
			okf.SeverityWarning, `"context" disagrees with its directory rules/`},
		{"matching singular kind", "context/note.md", conceptWithKind("context"), "", ""},
		{"matching plural kind", "rules/note.md", conceptWithKind("rules"), "", ""},
		{"resource kind is accepted", "skills/demo/references/note.md", conceptWithKind("skill-resource"), "", ""},
		{"no kind", "context/note.md", "---\ntype: Concept\n---\n\nText.\n", "", ""},
		{"directory that is not a kind", "notes/note.md", conceptWithKind("skill"), "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := writeTree(t, map[string]string{tt.path: tt.body})
			b, err := okf.Load(os.DirFS(dir))
			if err != nil {
				t.Fatal(err)
			}

			// Act
			got := KindFindings(b)

			// Assert
			if tt.wantSev == "" {
				if len(got) != 0 {
					t.Fatalf("findings = %v, want none", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("findings = %v, want one", got)
			}
			f := got[0]
			if f.Code != okf.CodeTypeInvalid || f.Severity != tt.wantSev || f.Path != tt.path || !strings.Contains(f.Message, tt.wantText) {
				t.Fatalf("finding = %+v, want %s %s at %s containing %q", f, okf.CodeTypeInvalid, tt.wantSev, tt.path, tt.wantText)
			}
		})
	}
}

func TestCheckIncludesKindFindings(t *testing.T) {
	// Arrange
	dir := writeTree(t, map[string]string{
		"index.md":         "---\nokf_version: \"0.2\"\n---\n\n# Subdirectories\n\n* [context](context/index.md) - Context\n",
		"context/index.md": "# Concepts\n\n* [Note](note.md)\n",
		"context/note.md":  conceptWithKind("bogus"),
	})
	b, err := okf.Load(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}

	// Act
	got := Check(b)

	// Assert
	found := false
	for _, f := range got {
		if f.Code == okf.CodeTypeInvalid && f.Severity == okf.SeverityError {
			found = true
		}
	}
	if !found {
		t.Fatalf("Check = %v, want an AR9B1 error for the unknown kind", got)
	}
}
