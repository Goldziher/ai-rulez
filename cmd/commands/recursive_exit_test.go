package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/progress"
	"github.com/spf13/viper"
)

const (
	validRootConfig  = "version = \"4.0\"\nname = \"ok\"\npresets = [\"claude\"]\ngitignore = false\n"
	brokenRootConfig = "version = \"4.0\"\nname = \"broken\"\npresets = [\n"
	profiledConfig   = "version = \"4.0\"\nname = \"profiled\"\npresets = [\"claude\"]\ngitignore = false\n\n[profiles]\nbackend = []\n"
)

// twoRoots builds a workspace with a valid root "a" and a second root "b" whose
// config is bConfig, and makes it the working directory.
func twoRoots(t *testing.T, bConfig string) (root string) {
	t.Helper()
	root = t.TempDir()
	writeFile(t, filepath.Join(root, "a", ".ai-rulez", "config.toml"), validRootConfig)
	writeFile(t, filepath.Join(root, "b", ".ai-rulez", "config.toml"), bConfig)
	chdir(t, root)

	progress.SetQuiet(true)
	t.Cleanup(func() {
		progress.SetQuiet(false)
		pluginMode, dryRun, profile, validateRecursive = false, false, "", false
	})
	pluginMode, dryRun, profile = false, false, ""
	return root
}

func TestRunRecursiveGenerate_ExitCode(t *testing.T) {
	tests := []struct {
		name         string
		bConfig      string
		dryRun       bool
		profile      string
		wantCode     int
		wantAWritten bool
	}{
		{name: "all valid", bConfig: validRootConfig, wantCode: 0, wantAWritten: true},
		{name: "broken nested root fails the run", bConfig: brokenRootConfig, wantCode: 1, wantAWritten: true},
		{name: "broken nested root fails dry-run", bConfig: brokenRootConfig, dryRun: true, wantCode: 1},
		{name: "profile missing from root a fails the run", bConfig: profiledConfig, profile: "backend", wantCode: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := twoRoots(t, tt.bConfig)
			dryRun, profile = tt.dryRun, tt.profile

			if got := runRecursiveGenerate(); got != tt.wantCode {
				t.Errorf("exit code = %d, want %d", got, tt.wantCode)
			}

			// The healthy root must be processed even though another root failed.
			_, err := os.Stat(filepath.Join(root, "a", "CLAUDE.md"))
			if written := err == nil; written != tt.wantAWritten {
				t.Errorf("root a generated = %v, want %v", written, tt.wantAWritten)
			}
		})
	}
}

// TestRecursiveGenerate_ProfileMissingInOneRoot pins --profile semantics across
// roots: a root that lacks the named profile is a hard error for that root (no
// silent fallback to default content), reported by path, while roots that do
// define it still generate.
func TestRecursiveGenerate_ProfileMissingInOneRoot(t *testing.T) {
	root := t.TempDir()
	// "a" has no profiles at all; "b" defines backend.
	writeFile(t, filepath.Join(root, "a", ".ai-rulez", "config.toml"), validRootConfig)
	writeFile(t, filepath.Join(root, "b", ".ai-rulez", "config.toml"), profiledConfig)
	chdir(t, root)
	progress.SetQuiet(true)
	t.Cleanup(func() { progress.SetQuiet(false); profile = "" })
	profile = "backend"

	configs := findConfigFilesRecursively()
	_, failed := processConfigFiles(configs)

	want := filepath.Join("a", ".ai-rulez", "config.toml")
	if len(failed) != 1 || failed[0] != want {
		t.Fatalf("failed = %v, want [%s]", failed, want)
	}
	if _, err := os.Stat(filepath.Join(root, "a", "CLAUDE.md")); err == nil {
		t.Error("root a lacks the profile but generated output anyway (silent fallback)")
	}
	if _, err := os.Stat(filepath.Join(root, "b", "CLAUDE.md")); err != nil {
		t.Errorf("root b defines the profile and must generate: %v", err)
	}
}

func TestRunRecursiveValidate_ExitCode(t *testing.T) {
	tests := []struct {
		name     string
		bConfig  string
		wantCode int
	}{
		{name: "all valid", bConfig: validRootConfig, wantCode: 0},
		{name: "broken nested root", bConfig: brokenRootConfig, wantCode: 1},
		{name: "structurally invalid root", bConfig: "version = \"4.0\"\nname = \"\"\npresets = [\"claude\"]\n", wantCode: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			twoRoots(t, tt.bConfig)
			if got := runRecursiveValidate(); got != tt.wantCode {
				t.Errorf("exit code = %d, want %d", got, tt.wantCode)
			}
		})
	}
}

func TestRunRecursiveValidate_NoConfigs(t *testing.T) {
	chdir(t, t.TempDir())
	progress.SetQuiet(true)
	t.Cleanup(func() { progress.SetQuiet(false) })
	if got := runRecursiveValidate(); got != 0 {
		t.Errorf("exit code = %d, want 0 when nothing is discovered", got)
	}
}

func TestValidateCmd_HonoursQuietFlag(t *testing.T) {
	twoRoots(t, validRootConfig)
	progress.SetQuiet(false)
	viper.Set("quiet", true)
	t.Cleanup(func() { viper.Set("quiet", false) })
	validateRecursive = true

	ValidateCmd.Run(ValidateCmd, nil)

	if !progress.IsQuiet() {
		t.Error("validate must apply --quiet to progress output")
	}
}
