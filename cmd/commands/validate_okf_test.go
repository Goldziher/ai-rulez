package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestOKFTreePath(t *testing.T) {
	for p, want := range map[string]bool{
		"index.md":                        true,
		"rules/go.md":                     true,
		"domains/web/rules/react.md":      true,
		"skills/review/SKILL.md":          true,
		"skills/review/references/doc.md": false,
		"commands/ship/assets/notes.md":   false,
		"local/rules/mine.md":             false,
		"README.md":                       false,
	} {
		if got := okfTreePath(p); got != want {
			t.Errorf("okfTreePath(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestValidateOKFTreeOnlyRunsOnABundle(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rules", "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{ConfigDir: dir}
	if _, _, ok := okfTreeFindings(cfg); ok {
		t.Fatal("a native tree without index.md must not be validated as OKF")
	}
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte("---\nokf_version: \"0.2\"\n---\n# Concepts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, findings, ok := okfTreeFindings(cfg)
	if !ok || len(findings) == 0 {
		t.Fatalf("a bundle with an untyped concept must report findings, got ok=%v %v", ok, findings)
	}
}
