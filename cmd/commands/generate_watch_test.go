package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/watch"
)

func resetWatchFlags(t *testing.T) {
	t.Helper()
	reset := func() {
		dryRun, generateCheck, userScope, pluginMode, recursive, generateWatch = false, false, false, false, false, false
	}
	reset()
	t.Cleanup(reset)
}

func TestCheckGenerateWatchFlags(t *testing.T) {
	tests := []struct {
		name    string
		set     func()
		wantErr bool
	}{
		{name: "alone", set: func() {}},
		{name: "dry-run", set: func() { dryRun = true }, wantErr: true},
		{name: "check", set: func() { generateCheck = true }, wantErr: true},
		{name: "user", set: func() { userScope = true }, wantErr: true},
		{name: "plugin", set: func() { pluginMode = true }, wantErr: true},
		{name: "recursive", set: func() { recursive = true }, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetWatchFlags(t)
			tt.set()
			if err := checkGenerateWatchFlags(); (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWatchTargets(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "proj")
	tests := []struct {
		name string
		cfg  *config.Config
		want []watch.Target
	}{
		{
			name: "config dir plus local include, remote include skipped",
			cfg: &config.Config{
				BaseDir: base, ConfigDir: filepath.Join(base, ".ai-rulez"), ConfigFile: "config.toml",
				Includes: []config.IncludeConfig{
					{Name: "shared", Source: "../shared"},
					{Name: "remote", Source: "https://github.com/acme/rules.git"},
					{Name: "abs", Source: filepath.Join(string(filepath.Separator), "opt", "rules")},
				},
			},
			want: []watch.Target{
				{Path: filepath.Join(base, ".ai-rulez")},
				{Path: filepath.Join(base, "..", "shared")},
				{Path: filepath.Join(string(filepath.Separator), "opt", "rules")},
			},
		},
		{
			name: "single-file config beside the project watches only the file",
			cfg:  &config.Config{BaseDir: base, ConfigDir: base, ConfigFile: "ai-rulez.yaml"},
			want: []watch.Target{{Path: filepath.Join(base, "ai-rulez.yaml"), File: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := watchTargets(tt.cfg, nil)
			if len(got) != len(tt.want) {
				t.Fatalf("targets = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("target %d = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestWatchTargets_FallbackWithoutConfig(t *testing.T) {
	resetWatchFlags(t)
	chdir(t, t.TempDir())
	got := watchTargets(nil, nil)
	if len(got) != 1 || got[0].Path != defaultConfigDirName || got[0].File {
		t.Errorf("fallback targets = %v, want the %s directory", got, defaultConfigDirName)
	}
}

func TestWatchIgnore(t *testing.T) {
	for path, want := range map[string]bool{
		"/p/.ai-rulez/.generated-manifest.json":       true,
		"/p/.ai-rulez/.generated-manifest.local.json": true,
		"/p/.ai-rulez/.gitignore":                     true,
		"/p/.ai-rulez/config.toml":                    false,
		"/p/.ai-rulez/rules/a.md":                     false,
	} {
		if got := watchIgnore(path); got != want {
			t.Errorf("watchIgnore(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestGenerateOnce_ReturnsLoadErrors(t *testing.T) {
	resetWatchFlags(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), brokenRootConfig)
	chdir(t, root)

	cfg, err := generateOnce(context.Background(), nil)

	if err == nil {
		t.Fatal("expected an error for a broken config")
	}
	if cfg != nil {
		t.Errorf("cfg = %v, want nil when the config does not load", cfg)
	}
}

func TestRunGenerateWatch_RegeneratesAndSurvivesBrokenConfig(t *testing.T) {
	resetWatchFlags(t)
	root := t.TempDir()
	cfgPath := filepath.Join(root, ".ai-rulez", "config.toml")
	writeFile(t, cfgPath, validRootConfig)
	chdir(t, root)
	rulePath := filepath.Join(root, ".ai-rulez", "rules", "watched.md")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runGenerateWatch(ctx, nil) }()

	waitFile := func(path string, contains string) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			if data, err := os.ReadFile(path); err == nil && (contains == "" || strings.Contains(string(data), contains)) {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s to contain %q", path, contains)
	}

	waitFile(filepath.Join(root, "CLAUDE.md"), "")

	// A broken config is logged and watching continues...
	writeFile(t, cfgPath, brokenRootConfig)
	time.Sleep(time.Second)
	// ...so the fix and a new rule are picked up afterwards.
	writeFile(t, cfgPath, validRootConfig)
	writeFile(t, rulePath, "# Watched\n\nwatch-marker-rule\n")
	waitFile(filepath.Join(root, ".claude", "rules", "watched.md"), "watch-marker-rule")

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runGenerateWatch: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("watch did not stop on cancellation")
	}
}

func TestDescribeTriggers_LeavesTheInputAlone(t *testing.T) {
	// Arrange
	abs, err := filepath.Abs(filepath.Join("rules", "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	triggers := []string{abs, "/elsewhere/b.md", "/elsewhere/c.md", "/elsewhere/d.md"}
	want := append([]string(nil), triggers...)

	// Act
	got := describeTriggers(triggers)

	// Assert
	if !reflect.DeepEqual(triggers, want) {
		t.Errorf("describeTriggers mutated its input: %v, want %v", triggers, want)
	}
	if !strings.Contains(got, filepath.Join("rules", "a.md")) || !strings.Contains(got, "(+1 more)") {
		t.Errorf("describeTriggers = %q", got)
	}
}

func TestChangedPaths_DoesNotDependOnSortOrder(t *testing.T) {
	tests := []struct {
		name     string
		triggers []string
		want     []string
	}{
		{name: "initial run", triggers: []string{"initial"}, want: nil},
		{name: "a path sorts before initial", triggers: []string{"/p/.ai-rulez/a.md", "initial"}, want: []string{"/p/.ai-rulez/a.md"}},
		{name: "only changes", triggers: []string{"/p/a.md", "/p/b.md"}, want: []string{"/p/a.md", "/p/b.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := changedPaths(tt.triggers); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("changedPaths(%v) = %v, want %v", tt.triggers, got, tt.want)
			}
		})
	}
}

func TestGeneratedOutputFilter_IgnoresRecordedOutputsOnly(t *testing.T) {
	// Arrange: an include source that also holds generated outputs.
	resetWatchFlags(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), validRootConfig)
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "r.md"), "# R\n\nbody\n")
	chdir(t, root)
	cfg, err := generateOnce(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	filter := newGeneratedOutputFilter()

	// Before the first refresh nothing is known, so nothing is filtered.
	if filter.ignore(filepath.Join(cfg.BaseDir, "CLAUDE.md")) {
		t.Fatal("no manifest consulted yet: nothing may be ignored")
	}

	// Act
	filter.refresh(cfg)

	// Assert
	if !filter.ignore(filepath.Join(cfg.BaseDir, "CLAUDE.md")) {
		t.Error("a recorded output must be ignored")
	}
	if filter.ignore(filepath.Join(cfg.BaseDir, ".ai-rulez", "rules", "r.md")) {
		t.Error("a source must still trigger a run")
	}
}

func TestInterruptContext_SecondSignalKills(t *testing.T) {
	if os.Getenv("AI_RULEZ_INTERRUPT_HELPER") == "1" {
		ctx, stop := interruptContext(context.Background())
		defer stop()
		fmt.Println("ready")
		<-ctx.Done()
		time.Sleep(200 * time.Millisecond) // interruptContext releases the handlers right after cancelling
		fmt.Println("cancelled")
		time.Sleep(time.Minute) // a run that does not stop on its own
		return
	}
	if runtime.GOOS == "windows" {
		t.Skip("os.Interrupt cannot be sent to a process on Windows")
	}
	// Arrange
	cmd := exec.Command(os.Args[0], "-test.run=^TestInterruptContext_SecondSignalKills$")
	cmd.Env = append(os.Environ(), "AI_RULEZ_INTERRUPT_HELPER=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	lines := bufio.NewScanner(out)
	waitLine := func(want string) {
		t.Helper()
		for lines.Scan() {
			if strings.TrimSpace(lines.Text()) == want {
				return
			}
		}
		t.Fatalf("helper never printed %q", want)
	}
	waitLine("ready")

	// Act: the first interrupt cancels, the second must kill.
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	waitLine("cancelled")
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}

	// Assert
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("helper exited cleanly; the second interrupt should have killed it")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the second interrupt did not stop the process")
	}
}
