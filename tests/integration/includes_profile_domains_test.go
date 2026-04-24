package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	_ "github.com/Goldziher/ai-rulez/internal/includes" // register includes resolver
)

// Integration-style reproduction of issue #97: profiles should accept domains provided by includes.
func TestIncludesProvideProfileDomains(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()

	// Shared include repo layout
	includeRoot := filepath.Join(baseDir, "shared", ".ai-rulez")
	must(os.MkdirAll(filepath.Join(includeRoot, "domains", "golang", "rules"), 0o755))
	must(os.WriteFile(filepath.Join(includeRoot, "domains", "golang", "rules", "golang-standards.md"), []byte("# Go Standards"), 0o644))

	must(os.MkdirAll(filepath.Join(includeRoot, "domains", "react", "rules"), 0o755))
	must(os.WriteFile(filepath.Join(includeRoot, "domains", "react", "rules", "react-standards.md"), []byte("# React Standards"), 0o644))

	must(os.WriteFile(filepath.Join(includeRoot, "config.yaml"), []byte(`version: "3.0"
name: shared
presets: [claude]
`), 0o644))

	// Consumer project
	localAIRulez := filepath.Join(baseDir, ".ai-rulez")
	must(os.MkdirAll(localAIRulez, 0o755))
	configYAML := `version: "3.0"
name: consumer
presets: [claude]
includes:
  - name: shared
    source: ./shared
    include: [rules, context, skills, agents]
profiles:
  full: [golang, react]
default: full
`
	must(os.WriteFile(filepath.Join(localAIRulez, "config.yaml"), []byte(configYAML), 0o644))

	cfg, err := config.LoadConfig(context.Background(), baseDir)
	must(err)

	if err := cfg.ValidateV3(); err != nil {
		t.Fatalf("validate failed: %v", err)
	}

	tree, err := cfg.GetContentForProfile("full")
	must(err)

	if _, ok := tree.Domains["golang"]; !ok {
		t.Fatalf("expected golang domain from include to be present")
	}
	if _, ok := tree.Domains["react"]; !ok {
		t.Fatalf("expected react domain from include to be present")
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
