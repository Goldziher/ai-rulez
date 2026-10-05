package commands

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// importingConfig loads a project that installs a skill with a hard-coded
// secret from a local directory outside its config, with the given scan_imports level.
func importingConfig(t *testing.T, level string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	vendor := filepath.ToSlash(filepath.Join(dir, "vendor", "imp"))
	security := ""
	if level != "" {
		security = "\n[lint.security]\nscan_imports = \"" + level + "\"\n"
	}
	writeFile(t, filepath.Join(dir, "proj", ".ai-rulez", "config.toml"),
		validRootConfig+"\n[[installed_skills]]\nname = \"imp\"\nsource = \""+vendor+"\"\npath = \".\"\n"+security)
	writeFile(t, filepath.Join(dir, "vendor", "imp", "SKILL.md"),
		"---\nname: imp\ndescription: Use when testing the imported content scan.\n---\nkey AKIAIOSFODNN7EXAMPLE\n")
	cfg, err := config.LoadConfig(context.Background(), filepath.Join(dir, "proj"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
