package okfbridge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/okf"
)

func TestRenderConceptWrapsNativeFrontmatter(t *testing.T) {
	got, err := RenderConcept(KindRule, "", "go", []byte("---\npriority: high\ntargets:\n  - claude\n---\n# Go\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := "---\ntype: Decision\ntitle: Go\nx-ai-rulez:\n  kind: rule\n  id: go\n  metadata:\n    priority: high\n    targets:\n      - claude\n---\n\n# Go\n"
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderConceptIsIdempotentAndKeepsOKFFiles(t *testing.T) {
	first, err := RenderConcept(KindContext, "web", "overview", []byte("Plain body\n"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderConcept(KindContext, "web", "overview", first)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("not idempotent:\n%s\n%s", first, second)
	}
	if !strings.Contains(string(first), "domain: web") || !strings.Contains(string(first), "type: Concept") {
		t.Fatalf("unexpected concept:\n%s", first)
	}
}

func TestRenderConceptRejectsMalformedFrontmatter(t *testing.T) {
	if _, err := RenderConcept(KindRule, "", "x", []byte("---\nkey: [unclosed\n---\nbody\n")); err == nil {
		t.Fatal("expected an error")
	}
}

func TestRenderedFilesExportByteForByte(t *testing.T) {
	dir := t.TempDir()
	put := func(rel string, kind Kind, domain, id, content string) {
		data, err := RenderConcept(kind, domain, id, []byte(content))
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("rules/go.md", KindRule, "", "go", "---\npriority: high\n---\n# Go\n")
	put("skills/review/SKILL.md", KindSkill, "", "review", "---\ndescription: \"Reviews code\"\npriority: medium\n---\n# Review\n")
	put("domains/web/context/ui.md", KindContext, "web", "ui", "# UI\n")
	if err := RefreshIndexes(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	tree, err := config.ScanContentTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Export(tree, ExportOptions{LocalDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	disk := readTree(t, dir)
	for _, f := range res.Files {
		if strings.HasPrefix(f.Path, "skills/review/") && f.Path != "skills/review/SKILL.md" && f.Path != "skills/review/index.md" {
			continue
		}
		if disk[f.Path] != string(f.Data) {
			t.Errorf("%s differs:\n%s\n--- export\n%s", f.Path, disk[f.Path], f.Data)
		}
	}
}

func TestRefreshIndexesListsContentAndDropsStaleOnes(t *testing.T) {
	dir := writeTree(t, map[string]string{})
	if err := RefreshIndexes(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	root, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil || !strings.Contains(string(root), "okf_version") {
		t.Fatalf("root index missing okf_version: %q %v", root, err)
	}
	data, err := RenderConcept(KindRule, "", "a", []byte("# A\n"))
	if err != nil {
		t.Fatal(err)
	}
	rule := filepath.Join(dir, "rules", "a.md")
	if err := os.MkdirAll(filepath.Dir(rule), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rule, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RefreshIndexes(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if got := readTree(t, dir)["rules/index.md"]; !strings.Contains(got, "[A](a.md)") {
		t.Fatalf("rules index does not list the rule:\n%s", got)
	}
	if err := os.Remove(rule); err != nil {
		t.Fatal(err)
	}
	if err := RefreshIndexes(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "index.md")); !os.IsNotExist(err) {
		t.Fatalf("stale rules/index.md was kept (err=%v)", err)
	}
	b, err := okf.Load(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	if f := append(b.CheckRoot(), b.Validate()...); len(f) != 0 {
		t.Fatalf("findings: %+v", f)
	}
}

// An index.md that holds prose is the user's, not a generated listing: before
// RefreshIndexes overwrites it, the original is copied to <dir>.bak-okf-<time>,
// as `migrate okf` does. Generated listings are rewritten without a copy.
func TestRefreshIndexesBacksUpAProseIndexBeforeOverwritingIt(t *testing.T) {
	// Arrange
	rule, err := RenderConcept(KindRule, "", "a", []byte("# A\n"))
	if err != nil {
		t.Fatal(err)
	}
	const prose = "# Rules\n\nNotes the team keeps here.\n"
	dir := writeTree(t, map[string]string{"rules/a.md": string(rule), "rules/index.md": prose})

	// Act
	if err := RefreshIndexes(context.Background(), dir); err != nil {
		t.Fatal(err)
	}

	// Assert
	backups, err := filepath.Glob(dir + ".bak-okf-*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("want one backup directory beside %s, got %v (err=%v)", dir, backups, err)
	}
	got, err := os.ReadFile(filepath.Join(backups[0], "rules", "index.md"))
	if err != nil || string(got) != prose {
		t.Fatalf("the prose index was not backed up: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(backups[0], "index.md")); !os.IsNotExist(err) {
		t.Fatalf("a new generated listing has nothing to back up (err=%v)", err)
	}

	// A second refresh only rewrites generated listings: no new backup.
	if err := RefreshIndexes(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if again, _ := filepath.Glob(dir + ".bak-okf-*"); len(again) != 1 {
		t.Fatalf("a refresh over generated listings took a backup: %v", again)
	}
}
