package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/tools/skillbench/internal/score"
)

func TestFileRewritesOnlyTheBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	startMarker, endMarker := markers(DefaultBlock)
	start := "before\n" + startMarker + "\nold\n" + endMarker + "\nafter\n"
	if err := os.WriteFile(path, []byte(start), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := File(path, DefaultBlock, "new", true); err == nil {
		t.Fatal("check must report a stale file")
	}
	if err := File(path, DefaultBlock, "new", false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "before\n" + startMarker + "\n\nnew\n\n" + endMarker + "\nafter\n"
	if string(got) != want {
		t.Fatalf("file = %q", got)
	}
	if err := File(path, DefaultBlock, "new", true); err != nil {
		t.Fatalf("a current file passes: %v", err)
	}
}

func TestFileWithoutMarkers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(path, []byte("none"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := File(path, DefaultBlock, "x", false); err == nil {
		t.Fatal("missing markers must fail")
	}
}

func TestTable(t *testing.T) {
	s := &score.Summary{Samples: 2, Malicious: 1, Benign: 1, Dataset: "D", Track: "test", Claim: "static",
		Tools: []score.ToolResult{{Tool: "ai-rulez", Version: "1", Rates: score.Rates{Recall: 0.5}}}}
	got := Table(s)
	for _, want := range []string{"| ai-rulez | 1 | 0.500", "static", "D source-disjoint `test`"} {
		if !strings.Contains(got, want) {
			t.Fatalf("table lacks %q:\n%s", want, got)
		}
	}
}

func TestNamedBlockLeavesTheOthers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	doc := "<!-- skillbench:start -->\nA\n<!-- skillbench:end -->\n<!-- skillbench-dev:start -->\nB\n<!-- skillbench-dev:end -->\n"
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := File(path, "skillbench-dev", "C", false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "\nA\n") || !strings.Contains(string(got), "\n\nC\n\n") || strings.Contains(string(got), "\nB\n") {
		t.Fatalf("file = %q", got)
	}
}

func TestBenignTable(t *testing.T) {
	s := &score.Summary{Samples: 10, Benign: 10, Track: "clean", Claim: "static",
		Tools: []score.ToolResult{{Tool: "ai-rulez", Version: "1", Counts: score.Counts{FP: 2}, Rates: score.Rates{FPR: 0.2}}}}
	got := Table(s)
	if !strings.Contains(got, "| ai-rulez | 1 | 2 of 10 | 20.0% (0.0-0.0) |") || strings.Contains(got, "Recall") {
		t.Fatalf("table = %s", got)
	}
}
