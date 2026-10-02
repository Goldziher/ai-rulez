package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/progress"
)

// manifestFileCount reads the generated manifest and returns how many files the
// run recorded writing. The manifest is ai-rulez's own record of the files it
// generated, so for a project with no merged settings document (which is written
// but deliberately not manifested) it is exactly the number a summary should
// report.
func manifestFileCount(t *testing.T, configDir string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(configDir, ".generated-manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest struct {
		Files []string `json:"files"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	return len(manifest.Files)
}

// TestProcessConfigFile_ReportsCountedFiles is the regression test for the
// fabricated progress total: processConfigFile returned len(cfg.Presets)*3, a
// number nothing measured, and runRecursiveGenerate printed it as
// "✅ Total: Generated %d file(s)". One preset and three skills generate four
// files (CLAUDE.md plus a SKILL.md each); the estimate said three.
func TestProcessConfigFile_ReportsCountedFiles(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".ai-rulez")
	writeFile(t, filepath.Join(configDir, "config.toml"),
		"version = \"4.0\"\nname = \"counted\"\npresets = [\"claude\"]\ngitignore = false\n")
	writeFile(t, filepath.Join(configDir, "rules", "style.md"),
		"---\npriority: high\n---\n# Style\n\nUse tabs.\n")
	for _, skill := range []string{"alpha", "beta", "gamma"} {
		writeFile(t, filepath.Join(configDir, "skills", skill, "SKILL.md"),
			"---\ndescription: "+skill+"\n---\nbody\n")
	}

	t.Cleanup(func() { pluginMode, dryRun, profile = false, false, "" })
	pluginMode, dryRun, profile = false, false, ""

	progress.SetQuiet(true)
	t.Cleanup(func() { progress.SetQuiet(false) })

	counter := progress.NewFileCounter(1, "Processing configurations")
	got, err := processConfigFile(filepath.Join(configDir, "config.toml"), counter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	counter.Finish()

	want := manifestFileCount(t, configDir)
	if want == 0 {
		t.Fatal("fixture generated nothing; the oracle is meaningless")
	}
	if got != want {
		t.Errorf("reported %d generated file(s), but %d were written", got, want)
	}
}

// TestProcessConfigFile_ReportsZeroOnFailure keeps a failed config out of the
// total: nothing was generated, so nothing may be counted.
func TestProcessConfigFile_ReportsZeroOnFailure(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, ".ai-rulez", "config.toml")
	writeFile(t, configPath, "version = \"4.0\"\nname = \"\"\npresets = [\"claude\"]\n")

	t.Cleanup(func() { pluginMode, dryRun, profile = false, false, "" })
	pluginMode, dryRun, profile = false, false, ""

	progress.SetQuiet(true)
	t.Cleanup(func() { progress.SetQuiet(false) })

	counter := progress.NewFileCounter(1, "Processing configurations")
	got, err := processConfigFile(configPath, counter)
	if err == nil {
		t.Error("a config that failed to load must return an error")
	}
	if got != 0 {
		t.Errorf("a config that failed to load reported %d generated file(s), want 0", got)
	}
	counter.Finish()
}
