package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/progress"
)

func TestRunDriftCheck_ExitCodes(t *testing.T) {
	tests := []struct {
		name      string
		recursive bool
		edit      string // file (relative to the workspace) to tamper with
		want      int
	}{
		{name: "clean", want: 0},
		{name: "recursive clean", recursive: true, want: 0},
		{name: "detects an edit", edit: "a/CLAUDE.md", want: exitDrift},
		{name: "recursive finds drift in a nested root", recursive: true, edit: "b/CLAUDE.md", want: exitDrift},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := twoRoots(t, validRootConfig)
			if code := runRecursiveGenerate(); code != 0 {
				t.Fatalf("setup generate exit code = %d", code)
			}
			if tt.edit != "" {
				f, err := os.OpenFile(filepath.Join(root, tt.edit), os.O_APPEND|os.O_WRONLY, 0o644)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.WriteString("hand edit\n"); err != nil {
					t.Fatal(err)
				}
				_ = f.Close()
			}
			progress.SetQuiet(true)
			if !tt.recursive {
				useConfigFile(t, filepath.Join(root, "a", ".ai-rulez", "config.toml"))
			}
			if got := runDriftCheck(tt.recursive); got != tt.want {
				t.Errorf("exit code = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRunDriftCheck_BrokenRootIsAnError(t *testing.T) {
	twoRoots(t, brokenRootConfig)
	if got := runDriftCheck(true); got != 1 {
		t.Errorf("exit code = %d, want 1 for an unloadable root", got)
	}
}

func TestCheckGenerateCheckFlags(t *testing.T) {
	defer func() { dryRun, pluginMode, updateGitignore = false, false, false }()
	for name, set := range map[string]func(){
		"dry-run":   func() { dryRun = true },
		"plugin":    func() { pluginMode = true },
		"gitignore": func() { updateGitignore = true },
	} {
		dryRun, pluginMode, updateGitignore = false, false, false
		set()
		if err := checkGenerateCheckFlags(); err == nil {
			t.Errorf("--check with %s must be rejected", name)
		}
	}
	dryRun, pluginMode, updateGitignore = false, false, false
	if err := checkGenerateCheckFlags(); err != nil {
		t.Errorf("--check alone must be accepted: %v", err)
	}
}
